package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

// Handler exposes WebRTC session attach over HTTP.
type Handler struct {
	sm  *engine.SessionManager
	rtc *Service
}

func NewHandler(sm *engine.SessionManager, service *Service) *Handler {
	return &Handler{sm: sm, rtc: service}
}

// Providers lists all registered WebRTC signaling providers and the default provider.
func (h *Handler) Providers(w http.ResponseWriter, _ *http.Request) error {
	type providerView struct {
		Name string `json:"name"`
		Metadata
	}

	providers := h.rtc.Registry().List()
	views := make([]providerView, 0, len(providers))
	for _, p := range providers {
		views = append(views, providerView{
			Name:     p.Name(),
			Metadata: p.Metadata(),
		})
	}

	data := map[string]any{
		"default":   h.rtc.Registry().Default(),
		"providers": views,
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(data))
}

// Attach negotiates or returns WebRTC credentials for one session.
// Provider can be specified via URL param /webrtc/{id}/{provider}, query param ?provider=..., or JSON body {"provider": "..."}.
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

	provider := chi.URLParam(request, "provider")
	if provider == "" {
		provider = request.URL.Query().Get("provider")
	}
	if provider == "" && request.Body != nil && request.ContentLength > 0 {
		var reqBody struct {
			Provider string `json:"provider"`
		}
		if jsonErr := json.NewDecoder(request.Body).Decode(&reqBody); jsonErr == nil && reqBody.Provider != "" {
			provider = reqBody.Provider
		}
	}

	credentials, err := h.rtc.Attach(request.Context(), session, provider)
	if err != nil {
		slog.ErrorContext(request.Context(), "webrtc session attach failed",
			"session_id", sessionID,
			"provider", provider,
			"error", err,
		)
		return attachError(err)
	}

	return httpx.JSON(w, http.StatusOK, result.SuccessData(credentials))
}

func attachError(err error) error {
	switch {
	case errors.Is(err, ErrServiceClosed):
		return errcode.New(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "WebRTC service is closed")
	case errors.Is(err, ErrChannelOpen):
		return errcode.New(http.StatusBadGateway, http.StatusBadGateway, "WebRTC channel cannot be opened")
	case errors.Is(err, ErrNegotiationFailed):
		return errcode.New(http.StatusBadGateway, http.StatusBadGateway, "WebRTC signaling negotiation failed")
	case errors.Is(err, ErrBusy):
		return errcode.New(http.StatusConflict, http.StatusConflict, "Session is busy")
	case errors.Is(err, ErrStaleSession):
		return errcode.New(http.StatusConflict, http.StatusConflict, "Session was replaced")
	case errors.Is(err, ErrInvalidSession):
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Invalid session")
	case errors.Is(err, ErrInvalidConfig):
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Invalid WebRTC configuration")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "WebRTC session canceled")
	default:
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, "Internal server error")
	}
}
