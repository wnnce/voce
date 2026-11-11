package webrtc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
)

func TestHandler_Attach_Validation(t *testing.T) {
	sm := engine.NewSessionManager(nil, nil, time.Minute)
	defer sm.Stop()

	handler := NewHandler(sm, nil)

	t.Run("invalid session uuid returns bad request", func(t *testing.T) {
		r := chi.NewRouter()
		var handlerErr error
		r.Post("/webrtc/{id}", func(w http.ResponseWriter, req *http.Request) {
			handlerErr = handler.Attach(w, req)
		})

		req := httptest.NewRequest(http.MethodPost, "/webrtc/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Error(t, handlerErr)
		var serverErr *errcode.ServerError
		require.ErrorAs(t, handlerErr, &serverErr)
		assert.Equal(t, http.StatusBadRequest, serverErr.Code)
	})

	t.Run("session not found returns 404", func(t *testing.T) {
		r := chi.NewRouter()
		var handlerErr error
		r.Post("/webrtc/{id}", func(w http.ResponseWriter, req *http.Request) {
			handlerErr = handler.Attach(w, req)
		})

		randomUUID := uuid.New().String()
		req := httptest.NewRequest(http.MethodPost, "/webrtc/"+randomUUID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Error(t, handlerErr)
		var serverErr *errcode.ServerError
		require.ErrorAs(t, handlerErr, &serverErr)
		assert.Equal(t, http.StatusNotFound, serverErr.Code)
	})
}

func TestAttachError_Mappings(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		expectCode int
	}{
		{
			name:       "service closed",
			err:        ErrServiceClosed,
			expectCode: http.StatusServiceUnavailable,
		},
		{
			name:       "channel open or negotiation failed",
			err:        ErrNegotiationFailed,
			expectCode: http.StatusBadGateway,
		},
		{
			name:       "session busy",
			err:        ErrBusy,
			expectCode: http.StatusConflict,
		},
		{
			name:       "invalid session",
			err:        ErrInvalidSession,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "context canceled",
			err:        context.Canceled,
			expectCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := attachError(tt.err)
			var serverErr *errcode.ServerError
			require.ErrorAs(t, mapped, &serverErr)
			assert.Equal(t, tt.expectCode, serverErr.Code)
		})
	}
}

func TestHandler_Providers(t *testing.T) {
	sm := engine.NewSessionManager(nil, nil, time.Minute)
	defer sm.Stop()

	reg := NewRegistry()
	require.NoError(t, reg.Register(&mockProvider{name: "wtn", label: "Volcengine WTN"}))
	svc, err := NewService(reg, sm)
	require.NoError(t, err)
	defer svc.Close()

	handler := NewHandler(sm, svc)
	req := httptest.NewRequest(http.MethodGet, "/webrtc/providers", nil)
	rec := httptest.NewRecorder()

	err = handler.Providers(rec, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"default":"wtn"`)
	assert.Contains(t, rec.Body.String(), `"name":"wtn"`)
}
