package gateway

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/result"
)

func TestHandlerHealthStateAndNoActiveMachine(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()
	active := testMachine("active", p, MachineStateActive, 1)
	suspended := testMachine("suspended", p, MachineStateSuspended, 0)
	h := NewHandler(&MachineManager{items: map[string]*Machine{active.ID: active, suspended.ID: suspended}}, newTestSessionManager())

	health := httptest.NewRecorder()
	require.NoError(t, h.HandleHealth(health, httptest.NewRequest(http.MethodGet, "/health", nil)))
	assert.Equal(t, http.StatusOK, health.Code)

	state := httptest.NewRecorder()
	require.NoError(t, h.HandleState(state, httptest.NewRequest(http.MethodGet, "/state", nil)))
	var response result.Result[StateResponse]
	require.NoError(t, json.Unmarshal(state.Body.Bytes(), &response))
	assert.Equal(t, 2, response.Data.Machines)
	assert.Equal(t, 1, response.Data.ActiveMachines)

	h.mm = &MachineManager{items: map[string]*Machine{suspended.ID: suspended}}
	err := h.ProxyToAny(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
}

func TestHandlerSessionLookupAndRegisterValidation(t *testing.T) {
	sm := newTestSessionManager()
	key := testSessionKey(1)
	sm.Store(NewSession(key, nil, nil))
	defer sm.Delete(key)
	h := NewHandler(nil, sm)

	session, err := h.getSessionFromRequest(requestWithSessionID(key.String()))
	require.NoError(t, err)
	assert.Equal(t, key, session.key)
	_, err = h.getSessionFromRequest(requestWithSessionID("invalid"))
	require.Error(t, err)
	_, err = h.getSessionFromRequest(requestWithSessionID(testSessionKey(2).String()))
	require.Error(t, err)

	err = h.HandleRegister(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/register?port=invalid", nil))
	require.Error(t, err)
}

func TestHandlerRealtimeAcquiresSessionBeforeUpgrade(t *testing.T) {
	sm := newTestSessionManager()
	key := testSessionKey(1)
	session := NewSession(key, nil, nil)
	sm.Store(session)
	defer sm.Delete(key)
	h := NewHandler(nil, sm)

	err := h.HandleRealtime(httptest.NewRecorder(), requestWithSessionID(key.String()))
	require.Error(t, err)
	assert.Equal(t, SessionIdle, session.State())

	require.True(t, session.Acquire())
	err = h.HandleRealtime(httptest.NewRecorder(), requestWithSessionID(key.String()))
	require.Error(t, err)
	assert.Equal(t, SessionPending, session.State())
}

func TestHandlerProxySessionLogic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session", r.URL.Path)
		assert.Equal(t, "value", r.URL.Query().Get("query"))
		assert.Equal(t, "request", r.Header.Get("X-Request"))
		w.Header().Set("X-Response", "response")
		if r.URL.Query().Get("status") == "created" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	machine := machineForServer(t, server.URL)
	h := NewHandler(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/session?query=value&status=created", nil)
	req.Header.Set("X-Request", "request")
	response := httptest.NewRecorder()
	called := false
	require.NoError(t, h.proxySessionLogic(response, req, machine, func([]byte) { called = true }))
	assert.False(t, called)
	assert.Equal(t, http.StatusCreated, response.Code)
	assert.Equal(t, "created", response.Body.String())
	assert.Equal(t, "response", response.Header().Get("X-Response"))

	req = httptest.NewRequest(http.MethodPost, "/session?query=value", nil)
	req.Header.Set("X-Request", "request")
	response = httptest.NewRecorder()
	require.NoError(t, h.proxySessionLogic(response, req, machine, func(body []byte) {
		called = string(body) == "ok"
	}))
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestHandlerSessionLifecycleProxy(t *testing.T) {
	key := testSessionKey(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"session_id":"` + key.String() + `"}}`))
	}))
	defer server.Close()
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()
	machine := machineForServer(t, server.URL)
	machine.Pool = p
	machine.state.Store(int32(MachineStateActive))
	sm := newTestSessionManager()
	manager := &MachineManager{items: map[string]*Machine{machine.ID: machine}}
	h := NewHandler(manager, sm)

	create := httptest.NewRecorder()
	require.NoError(t, h.HandleSessionCreate(create, httptest.NewRequest(http.MethodPost, "/sessions", nil)))
	session, exists := sm.Load(key)
	require.True(t, exists)
	require.Same(t, machine, session.machine)
	assert.Equal(t, int32(1), machine.Sessions())

	session.lastActiveAt.Store(time.Now().Add(-time.Second).UnixMilli())
	renew := httptest.NewRecorder()
	require.NoError(t, h.HandleSessionRenew(renew, requestWithSessionID(key.String())))
	assert.Greater(t, session.lastActiveAt.Load(), time.Now().Add(-time.Second).UnixMilli())

	deleteResponse := httptest.NewRecorder()
	require.NoError(t, h.HandleSessionDelete(deleteResponse, requestWithSessionID(key.String())))
	_, exists = sm.Load(key)
	assert.False(t, exists)
	assert.Zero(t, machine.Sessions())
}

func TestCopyHeader(t *testing.T) {
	src := http.Header{"X-Test": {"one", "two"}}
	dst := make(http.Header)
	copyHeader(src, dst)
	assert.Equal(t, []string{"one", "two"}, dst.Values("X-Test"))
}

func requestWithSessionID(id string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/sessions/"+id, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func machineForServer(t *testing.T, serverURL string) *Machine {
	t.Helper()
	u, err := url.Parse(serverURL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	value, err := strconv.Atoi(port)
	require.NoError(t, err)
	return &Machine{ID: "machine", Host: host, Port: value, sessions: make(map[protocol.SessionKey]struct{})}
}
