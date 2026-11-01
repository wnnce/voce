package agora

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

// Handler exposes Agora session attach over HTTP. It lives next to the service
// it serves, so the cgo dependency stays out of the shared handler package.
type Handler struct {
	sm  *engine.SessionManager
	rtc *Service
}

func NewHandler(sm *engine.SessionManager, service *Service) *Handler {
	return &Handler{sm: sm, rtc: service}
}

type attachResponse struct {
	AppID    string `json:"app_id"`
	Channel  string `json:"channel"`
	UID      string `json:"uid"`
	RTCToken string `json:"rtc_token"`
	RTMToken string `json:"rtm_token"`
	ExpireAt int64  `json:"expire_at"`
}

// Attach returns the Agora credentials for one session. Attaching twice returns
// the same binding's credentials until they expire.
func (h *Handler) Attach(w http.ResponseWriter, request *http.Request) error {
	sessionID := chi.URLParam(request, "id")
	sessionKey, err := protocol.ParseSessionKey(sessionID)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid session id")
	}
	session, ok := h.sm.LoadSession(sessionKey)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	switch session.Workflow.State() {
	case engine.WorkflowStateRunning, engine.WorkflowStatePaused:
	default:
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Session is not available")
	}

	credentials, err := h.rtc.Attach(request.Context(), session)
	if err != nil {
		return attachError(err)
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(attachResponse{
		AppID:    credentials.AppID,
		Channel:  credentials.Channel,
		UID:      credentials.UID,
		RTCToken: credentials.RTCToken,
		RTMToken: credentials.RTMToken,
		ExpireAt: credentials.ExpireAt.Unix(),
	}))
}

// attachError maps attach failures onto the HTTP contract so a client can
// tell "retry later" apart from "this session is unusable".
func attachError(err error) error {
	switch {
	case errors.Is(err, ErrServiceClosed):
		return errcode.New(http.StatusServiceUnavailable, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrEngineInit):
		return errcode.New(http.StatusServiceUnavailable, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrChannelOpen):
		return errcode.New(http.StatusBadGateway, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStaleSession):
		return errcode.New(http.StatusConflict, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidSession), errors.Is(err, ErrInvalidConfig):
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errcode.New(http.StatusNotFound, http.StatusNotFound, err.Error())
	default:
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, err.Error())
	}
}
