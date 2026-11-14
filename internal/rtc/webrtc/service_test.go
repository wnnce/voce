package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
)

type mockProvider struct {
	name            string
	label           string
	description     string
	negotiateCalled bool
	teardownCalled  bool
}

func (m *mockProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockProvider) Metadata() Metadata {
	label := m.label
	if label == "" {
		if m.name != "" {
			label = m.name
		} else {
			label = "Mock Provider"
		}
	}
	desc := m.description
	if desc == "" {
		desc = "mock description"
	}
	return Metadata{
		Label:       label,
		Description: desc,
	}
}

func (m *mockProvider) ICEServers(context.Context, protocol.SessionKey) ([]media.ICEConfig, error) {
	return nil, nil
}

func (m *mockProvider) Negotiate(ctx context.Context, req NegotiateRequest) (*NegotiateResult, error) {
	m.negotiateCalled = true
	return &NegotiateResult{
		AnswerSDP:   req.OfferSDP, // self-answer for mock
		Handle:      "mock-handle",
		Credentials: map[string]string{"type": "mock"},
	}, nil
}

func (m *mockProvider) Teardown(ctx context.Context, handle string) error {
	m.teardownCalled = true
	return nil
}

func TestService_NewServiceValidation(t *testing.T) {
	sm := engine.NewSessionManager(nil, nil, time.Minute)
	defer sm.Stop()

	t.Run("nil registry returns error", func(t *testing.T) {
		_, err := NewService(nil, sm)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("empty registry returns error", func(t *testing.T) {
		reg := NewRegistry()
		_, err := NewService(reg, sm)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("nil session manager returns error", func(t *testing.T) {
		_, err := NewServiceWithProvider(&mockProvider{}, nil)
		assert.ErrorIs(t, err, ErrInvalidSession)
	})

	t.Run("valid service creates successfully", func(t *testing.T) {
		svc, err := NewServiceWithProvider(&mockProvider{}, sm)
		require.NoError(t, err)
		assert.NotNil(t, svc)
		svc.Close()
	})
}

func TestService_AttachAfterClose(t *testing.T) {
	sm := engine.NewSessionManager(nil, nil, time.Minute)
	defer sm.Stop()

	svc, err := NewServiceWithProvider(&mockProvider{}, sm)
	require.NoError(t, err)

	svc.Close()
	svc.Close() // multiple Close calls should be safely idempotent via CAS

	_, err = svc.Attach(context.Background(), &engine.Session{Key: protocol.SessionKey{1, 2, 3, 4}})
	assert.ErrorIs(t, err, ErrServiceClosed)
}

func TestService_MultiProviderRouting(t *testing.T) {
	sm := engine.NewSessionManager(nil, nil, time.Minute)
	defer sm.Stop()

	reg := NewRegistry()
	p1 := &mockProvider{name: "mock1"}
	p2 := &mockProvider{name: "mock2"}
	require.NoError(t, reg.Register(p1))
	require.NoError(t, reg.Register(p2))
	assert.Equal(t, "mock1", reg.Default())

	svc, err := NewService(reg, sm)
	require.NoError(t, err)
	defer svc.Close()

	assert.Len(t, svc.Registry().List(), 2)

	// Unknown provider returns error
	_, err = svc.Attach(context.Background(), &engine.Session{Key: protocol.SessionKey{1, 2, 3, 4}}, "nonexistent")
	assert.ErrorIs(t, err, ErrInvalidConfig)
}
