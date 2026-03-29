package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type SessionMetadata struct {
	Language   string `json:"language,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	DeviceCode string `json:"device_code,omitempty"`
	IP         string `json:"ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
}

type CreateSessionRequest struct {
	Name       string                     `json:"name"`
	SessionID  string                     `json:"session_id,omitempty"`
	Metadata   *SessionMetadata           `json:"metadata,omitempty"`
	Properties map[string]json.RawMessage `json:"properties,omitempty"`
}

type HealthResponse struct {
	ActiveTime int64     `json:"active_time"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
}

func CreateWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	var payload CreateSessionRequest
	if err := httpx.BindJSON(request, &payload); err != nil {
		return err
	}
	manager := engine.DefaultSessionManager
	sessionId := strings.TrimSpace(payload.SessionID)
	if sessionId == "" {
		sessionId = uuid.New().String()
	}
	if _, ok := manager.LoadSession(sessionId); ok {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Session is exist")
	}
	sessionCtx := context.WithValue(manager.Context(), metadata.ContextTraceKey, sessionId)
	_, err := manager.CreateSession(sessionCtx, sessionId, payload.Name, payload.Properties)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(map[string]any{
		"session_id": sessionId,
	}))
}

func WorkflowSessionHealth(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	session, ok := engine.DefaultSessionManager.LoadSession(sessionId)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	var state string
	switch session.Workflow.State() {
	case engine.WorkflowStatePending:
		state = "pending"
	case engine.WorkflowStateStarting:
		state = "starting"
	case engine.WorkflowStateRunning:
		state = "running"
	case engine.WorkflowStatePaused:
		state = "paused"
	case engine.WorkflowStateStopped:
		state = "stopped"
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(HealthResponse{
		ActiveTime: session.LastActive.Load(),
		State:      state,
		CreatedAt:  session.CreatedAt,
	}))
}

func RenewWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	session, ok := engine.DefaultSessionManager.LoadSession(sessionId)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	session.UpdateActivity()
	return httpx.JSON(w, http.StatusOK, result.Success())
}

func DeleteWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	if _, ok := engine.DefaultSessionManager.LoadSession(sessionId); !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	engine.DefaultSessionManager.RemoveSession(sessionId)
	return httpx.JSON(w, http.StatusOK, result.Success())
}
