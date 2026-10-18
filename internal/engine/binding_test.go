package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

type sessionAudioProbe struct {
	BuiltinPlugin
	seen chan []byte
}

func (p *sessionAudioProbe) OnAudio(_ context.Context, _ Flow, audio schema.Audio) {
	p.seen <- append([]byte(nil), audio.Bytes()...)
}

type sessionOutboundProbe struct {
	writes atomic.Int32
	seen   chan protocol.PacketType
}

func (p *sessionOutboundProbe) writePacket(packet *protocol.Packet) error {
	p.writes.Add(1)
	p.seen <- packet.Type
	return nil
}

func TestSessionLeaseOwnership(t *testing.T) {
	session := &Session{}

	first, err := session.Bind()
	require.NoError(t, err)
	second, err := session.Bind()
	require.ErrorIs(t, err, ErrSessionBusy)
	assert.Zero(t, second)

	assert.False(t, session.Unbind(second, false))
	assert.True(t, session.Unbind(first, false))
	assert.False(t, session.Unbind(first, false))

	third, err := session.Bind()
	require.NoError(t, err)
	assert.Greater(t, third, first)
	assert.True(t, session.Unbind(third, false))
}

func TestSessionInboundAudio(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := make(chan []byte, 1)
	plugin := &sessionAudioProbe{seen: seen}
	node := newLoopNode(ctx, "head", plugin)
	require.NoError(t, node.Start())

	wf := &Workflow{
		ctx:      ctx,
		output:   make(chan *protocol.Packet, 1),
		nodes:    []Node{node},
		indexMap: map[string]int{"head": 0},
		graph:    &Graph{Config: &WorkflowConfig{Head: "head"}},
	}
	wf.state.Store(int32(WorkflowStateRunning))
	defer wf.Stop()

	session := &Session{Workflow: wf}
	lease, err := session.Bind()
	require.NoError(t, err)
	payload := []byte{1, 2, 3, 4}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeAudio
	packet.SetPayload(payload)
	require.NoError(t, session.ReceivePacket(lease, packet))

	select {
	case got := <-seen:
		assert.Equal(t, payload, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for audio input")
	}

	assert.True(t, session.Unbind(lease, false))
	assert.ErrorIs(t, session.ReceivePacket(lease, packet), ErrLeaseInvalid)
}

func TestSessionOutboundSingleConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wf := &Workflow{ctx: ctx, output: make(chan *protocol.Packet, 2)}
	wf.state.Store(int32(WorkflowStateRunning))
	session := &Session{Workflow: wf}

	out := &sessionOutboundProbe{seen: make(chan protocol.PacketType, 1)}
	lease, err := session.Bind()
	require.NoError(t, err)
	require.True(t, session.SetWriter(lease, PacketWriter(out.writePacket)))

	packet := protocol.AcquirePacket()
	packet.Type = protocol.TypeAudio
	wf.output <- packet
	select {
	case typ := <-out.seen:
		assert.Equal(t, protocol.TypeAudio, typ)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound packet")
	}

	_, err = session.Bind()
	require.ErrorIs(t, err, ErrSessionBusy)
	assert.True(t, session.Unbind(lease, false))

	second, err := session.Bind()
	require.NoError(t, err)
	assert.True(t, session.Unbind(second, false))
}

func BenchmarkSessionInboundAudio(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := newLoopNode(ctx, "head", &BuiltinPlugin{})
	wf := &Workflow{
		ctx:      ctx,
		output:   make(chan *protocol.Packet, 1),
		nodes:    []Node{node},
		indexMap: map[string]int{"head": 0},
		graph:    &Graph{Config: &WorkflowConfig{Head: "head"}},
	}
	wf.state.Store(int32(WorkflowStateRunning))
	session := &Session{Workflow: wf}
	lease, err := session.Bind()
	if err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, AudioFrameSize)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		packet := protocol.AcquirePacket()
		packet.Type = protocol.TypeAudio
		packet.SetPayload(payload)
		if err := session.ReceivePacket(lease, packet); err != nil {
			protocol.ReleasePacket(packet)
			b.Fatal(err)
		}
		protocol.ReleasePacket(packet)
	}
	b.StopTimer()
	if !session.Unbind(lease, false) {
		b.Fatal("failed to detach session")
	}
	_ = node
}
