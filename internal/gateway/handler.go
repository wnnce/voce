package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/wnnce/voce/biz/handler"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

// Handler handles incoming HTTP and WebSocket requests from clients and backend machines.
type Handler struct {
	mm *MachineManager
	sm *SessionManager
}

func NewHandler(mm *MachineManager, sm *SessionManager) *Handler {
	return &Handler{
		mm: mm, sm: sm,
	}
}

type StateResponse struct {
	Machines          int               `json:"machines"`
	ActiveMachines    int               `json:"active_machines"`
	ClientConnections int64             `json:"client_connections"`
	Sessions          int64             `json:"sessions"`
	MachineStates     []MachineSnapshot `json:"machine_states"`
}

func (h *Handler) HandleHealth(w http.ResponseWriter, _ *http.Request) error {
	return httpx.JSON(w, http.StatusOK, result.Success())
}

func (h *Handler) HandleState(w http.ResponseWriter, _ *http.Request) error {
	machines := make([]MachineSnapshot, 0)
	activeMachines := 0
	h.mm.RangeMachines(func(_ string, machine *Machine) bool {
		if machine.State() == MachineStateActive {
			activeMachines++
		}
		machines = append(machines, machine.Snapshot())
		return true
	})
	return httpx.JSON(w, http.StatusOK, result.SuccessData(StateResponse{
		Machines:          len(machines),
		ActiveMachines:    activeMachines,
		ClientConnections: clientConnections.Load(),
		Sessions:          h.sm.Count(),
		MachineStates:     machines,
	}))
}

func (h *Handler) ProxyToAny(w http.ResponseWriter, r *http.Request) error {
	machine := h.mm.Random()
	if machine == nil {
		return errcode.New(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "no active machines")
	}
	h.proxyRequest(w, r, machine)
	return nil
}

func (h *Handler) HandleSessionCreate(w http.ResponseWriter, r *http.Request) error {
	machine := h.mm.LeastSessions()
	if machine == nil {
		return errcode.New(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "no active machines")
	}

	return h.proxySessionLogic(w, r, machine, func(body []byte) {
		var res result.Result[map[string]string]
		_ = sonic.Unmarshal(body, &res)

		sid := res.Data["session_id"]
		key, _ := parseSessionKey(sid)
		binding := machine.Pool.Bind(key)
		session := NewSession(key, binding, machine)
		h.sm.Store(session)
		machine.AddSession(key)
		slog.Info("session registered on gateway", "id", sid, "machine", machine.ID, "addr", machine.Address())
	})
}

func (h *Handler) HandleSessionHealth(w http.ResponseWriter, r *http.Request) error {
	session, err := h.getSessionFromRequest(r)
	if err != nil {
		return err
	}
	h.proxyRequest(w, r, session.machine)
	return nil
}

func (h *Handler) HandleSessionRenew(w http.ResponseWriter, r *http.Request) error {
	session, err := h.getSessionFromRequest(r)
	if err != nil {
		return err
	}

	return h.proxySessionLogic(w, r, session.machine, func(_ []byte) {
		session.lastActiveAt.Store(time.Now().UnixMilli())
	})
}

func (h *Handler) HandleSessionDelete(w http.ResponseWriter, r *http.Request) error {
	session, err := h.getSessionFromRequest(r)
	if err != nil {
		return err
	}

	return h.proxySessionLogic(w, r, session.machine, func(_ []byte) {
		h.sm.Delete(session.key)
		slog.Info("session deleted manually", "sessionID", chi.URLParam(r, "id"))
	})
}

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		id = uuid.New().String()
	}
	host := q.Get("host")
	if host == "" {
		host = httpx.ClientIP(r)
	}
	port := defaultMachinePort
	if portStr := q.Get("port"); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return errcode.NewBadRequest("invalid port")
		}
		port = p
	}
	slog.Info("handling machine register request", "id", id, "host", host, "port", port)
	machine, err := h.mm.AcquireMachine(id, host, port)
	if err != nil {
		slog.Error("failed to acquire/register machine", "id", id, "error", err)
		return err
	}
	upgrader := websocket.NewUpgrader()
	upgrader.OnOpen(machine.OnOpen)
	upgrader.SetPongHandler(machine.OnPong)
	upgrader.OnMessage(machine.OnMessage)
	upgrader.OnClose(machine.OnClose)
	if _, err = upgrader.Upgrade(w, r, nil); err != nil {
		slog.Error("upgrade machine control socket failed", "id", id, "error", err)
		return errcode.NewInternal(err.Error())
	}
	slog.Info("machine control socket upgraded successfully", "id", id, "addr", machine.Address())
	return nil
}

func (h *Handler) HandleRealtime(w http.ResponseWriter, r *http.Request) error {
	session, err := h.getSessionFromRequest(r)
	if err != nil {
		return err
	}
	if !session.Acquire() {
		return errcode.New(http.StatusConflict, http.StatusConflict, "session is already connected")
	}

	upgrader := websocket.NewUpgrader()
	upgrader.OnOpen(session.OnClientOpen)
	upgrader.OnMessage(session.OnClientMessage)
	upgrader.OnClose(session.OnClientClose)
	upgrader.SetPingHandler(session.OnClientPing)
	if _, err = upgrader.Upgrade(w, r, nil); err != nil {
		session.Release()
		slog.Error("upgrade realtime client socket failed", "session", chi.URLParam(r, "id"), "error", err)
		return errcode.NewInternal(err.Error())
	}
	slog.Info("realtime client socket upgraded successfully", "session", chi.URLParam(r, "id"))
	return nil
}

func (h *Handler) WebHandler(w http.ResponseWriter, r *http.Request) {
	handler.WebHandler(w, r)
}

func (h *Handler) getSessionFromRequest(r *http.Request) (*Session, error) {
	id := chi.URLParam(r, "id")
	key, err := parseSessionKey(id)
	if err != nil {
		return nil, err
	}
	session, ok := h.sm.Load(key)
	if !ok {
		return nil, errcode.New(http.StatusNotFound, http.StatusNotFound, "session not found")
	}
	return session, nil
}

func (h *Handler) proxyRequest(w http.ResponseWriter, r *http.Request, machine *Machine) {
	target, _ := url.Parse("http://" + machine.Address())
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

func (h *Handler) proxySessionLogic(w http.ResponseWriter, r *http.Request, machine *Machine, onSuccess func(body []byte)) error {
	targetURL := "http://" + machine.Address() + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		return errcode.NewInternal(err.Error())
	}
	copyHeader(r.Header, req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errcode.NewInternal(err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errcode.NewInternal(err.Error())
	}

	if resp.StatusCode == http.StatusOK {
		onSuccess(body)
	}

	copyHeader(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return nil
}

func parseSessionKey(id string) (protocol.SessionKey, error) {
	key, err := protocol.ParseSessionKey(id)
	if err != nil {
		return key, errcode.NewBadRequest("invalid session id")
	}
	return key, nil
}

func copyHeader(src, dst http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
