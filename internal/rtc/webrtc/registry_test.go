package webrtc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
)

type dummyProvider struct {
	name        string
	label       string
	description string
}

func (d *dummyProvider) Name() string { return d.name }
func (d *dummyProvider) Metadata() Metadata {
	return Metadata{
		Label:       d.label,
		Description: d.description,
	}
}
func (d *dummyProvider) Negotiate(context.Context, NegotiateRequest) (*NegotiateResult, error) {
	return nil, nil
}
func (d *dummyProvider) ICEServers(context.Context, protocol.SessionKey) ([]media.ICEConfig, error) {
	return nil, nil
}
func (d *dummyProvider) Teardown(context.Context, string) error {
	return nil
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	assert.Equal(t, 0, reg.Len())
	assert.Empty(t, reg.Default())

	t.Run("nil provider returns error", func(t *testing.T) {
		err := reg.Register(nil)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("empty name returns error", func(t *testing.T) {
		err := reg.Register(&dummyProvider{name: ""})
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	p1 := &dummyProvider{name: "prov1", label: "Provider One", description: "Desc 1"}
	p2 := &dummyProvider{name: "prov2", label: "Provider Two", description: "Desc 2"}

	require.NoError(t, reg.Register(p1))
	assert.Equal(t, 1, reg.Len())
	assert.Equal(t, "prov1", reg.Default()) // First registered is automatic default

	require.NoError(t, reg.Register(p2))
	assert.Equal(t, 2, reg.Len())
	assert.Equal(t, "prov1", reg.Default()) // Default doesn't change on second register

	// Get default
	gotDefault, ok := reg.Get("")
	require.True(t, ok)
	assert.Equal(t, "prov1", gotDefault.Name())

	// Get specific
	gotP2, ok := reg.Get("prov2")
	require.True(t, ok)
	assert.Equal(t, "prov2", gotP2.Name())
	assert.Equal(t, "Provider Two", gotP2.Metadata().Label)

	// Get unknown
	_, ok = reg.Get("unknown")
	assert.False(t, ok)

	// List in order
	list := reg.List()
	require.Len(t, list, 2)
	assert.Equal(t, "prov1", list[0].Name())
	assert.Equal(t, "prov2", list[1].Name())

	// SetDefault to existing
	require.NoError(t, reg.SetDefault("prov2"))
	assert.Equal(t, "prov2", reg.Default())
	gotNewDefault, ok := reg.Get("")
	require.True(t, ok)
	assert.Equal(t, "prov2", gotNewDefault.Name())

	// SetDefault to unknown returns error
	err := reg.SetDefault("prov_unknown")
	assert.ErrorIs(t, err, ErrInvalidConfig)
}
