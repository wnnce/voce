package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

const (
	singleHandlerDeadline = 100
)

// askSignal wraps a schema.Signal together with a result Collector.
//
// It is the transport-layer envelope used by AskSignal: because it embeds the
// schema.Signal interface it satisfies schema.Signal itself and can travel
// through the existing signal channel / scheduler queue unchanged.
//
// The receiving node unwraps it in processEvent and hands the bare Signal to
// OnSignal, so if a downstream plugin forwards the signal further (SendSignal),
// it forwards the plain signal and the collector does NOT propagate. Aggregation
// stops at the direct downstream nodes; deeper aggregation is each node's own
// responsibility via its own AskSignal call.
type askSignal struct {
	schema.Signal
	collector *syncx.Collector[schema.Result]
}

type routeTable struct {
	signals  []Node
	payloads []Node
	audios   []Node
	videos   []Node
}

// finalizeDroppedAsk closes an askSignal's collector slot when the signal is
// dropped before reaching OnSignal (node stopped, context canceled, queue
// drained on shutdown, or enqueue failure). Without this the asking side would
// block forever waiting for a result that will never arrive. It is a no-op for
// plain signals and other event types.
func finalizeDroppedAsk(data schema.ReadOnly) {
	if as, ok := data.(*askSignal); ok {
		_ = as.collector.Done()
	}
}

// Node defines the polymorphic runtime instance of a plugin in the workflow graph.
type Node interface {
	Flow
	Start() error
	Ready()
	Stop()
	Pause()
	Resume()
	Input(data schema.ReadOnly)
	Context() context.Context
	Name() string

	addNextNode(event EventType, nextNode Node)
	addNextPortNode(event EventType, nextNode Node, port int)
	setSocketWriter(writer SocketWriter)

	// Internal scheduling methods
	processEvent(value schema.ReadOnly, useDeadline bool)
	processControl(ctrl controlType)
}

// baseNode contains the common logic for routing, socket writing, and properties.
type baseNode struct {
	ctx       context.Context
	plugin    Plugin
	name      string
	table     routeTable
	portTable [MaxPortCount]routeTable
	running   atomic.Bool
	writer    SocketWriter
}

func newBaseNode(ctx context.Context, name string, plugin Plugin) baseNode {
	return baseNode{
		ctx:    context.WithValue(ctx, metadata.ContextNodeNameKey, name),
		plugin: plugin,
		name:   name,
	}
}

func (n *baseNode) Context() context.Context {
	return n.ctx
}

func (n *baseNode) Name() string {
	return n.name
}

func (n *baseNode) setSocketWriter(writer SocketWriter) {
	n.writer = writer
}

func (n *baseNode) Ready() {
	if !n.running.Load() {
		return
	}
	n.plugin.OnReady(n.ctx, n)
}

func (n *baseNode) addNextNode(event EventType, nextNode Node) {
	switch event {
	case EventSignal:
		if !slices.Contains(n.table.signals, nextNode) {
			n.table.signals = append(n.table.signals, nextNode)
		}
	case EventPayload:
		if !slices.Contains(n.table.payloads, nextNode) {
			n.table.payloads = append(n.table.payloads, nextNode)
		}
	case EventAudio:
		if !slices.Contains(n.table.audios, nextNode) {
			n.table.audios = append(n.table.audios, nextNode)
		}
	case EventVideo:
		if !slices.Contains(n.table.videos, nextNode) {
			n.table.videos = append(n.table.videos, nextNode)
		}
	}
}

func (n *baseNode) addNextPortNode(event EventType, nextNode Node, port int) {
	if port < 0 || port >= MaxPortCount {
		return
	}
	switch event {
	case EventSignal:
		if !slices.Contains(n.portTable[port].signals, nextNode) {
			n.portTable[port].signals = append(n.portTable[port].signals, nextNode)
		}
		if !slices.Contains(n.table.signals, nextNode) {
			n.table.signals = append(n.table.signals, nextNode)
		}
	case EventPayload:
		if !slices.Contains(n.portTable[port].payloads, nextNode) {
			n.portTable[port].payloads = append(n.portTable[port].payloads, nextNode)
		}
		if !slices.Contains(n.table.payloads, nextNode) {
			n.table.payloads = append(n.table.payloads, nextNode)
		}
	case EventAudio:
		if !slices.Contains(n.portTable[port].audios, nextNode) {
			n.portTable[port].audios = append(n.portTable[port].audios, nextNode)
		}
		if !slices.Contains(n.table.audios, nextNode) {
			n.table.audios = append(n.table.audios, nextNode)
		}
	case EventVideo:
		if !slices.Contains(n.portTable[port].videos, nextNode) {
			n.portTable[port].videos = append(n.portTable[port].videos, nextNode)
		}
		if !slices.Contains(n.table.videos, nextNode) {
			n.table.videos = append(n.table.videos, nextNode)
		}
	}
}

func (n *baseNode) SendSignal(value schema.Signal) {
	if len(n.table.signals) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, next := range n.table.signals {
		next.Input(value)
	}
}

func (n *baseNode) SendSignalToPort(port int, value schema.Signal) {
	if port < 0 || port >= MaxPortCount || len(n.portTable[port].signals) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, next := range n.portTable[port].signals {
		next.Input(value)
	}
}

func (n *baseNode) AskSignal(value schema.Signal) *syncx.Collector[schema.Result] {
	return n.askSignal(n.table.signals, value)
}

func (n *baseNode) AskSignalToPort(port int, value schema.Signal) *syncx.Collector[schema.Result] {
	if port < 0 || port >= MaxPortCount {
		return syncx.NewCollector[schema.Result](0)
	}
	return n.askSignal(n.portTable[port].signals, value)
}

// askSignal fans the signal out to the given direct downstream nodes, wrapping
// it so each downstream's OnSignal result is collected. The returned collector
// closes once every downstream has reported (via processEvent's deferred Done),
// even if some downstreams are stopped or panic.
//
// NOTE: In worker-pool scheduler mode the returned collector MUST be consumed
// asynchronously (from a separate goroutine), never blocked-on inside the same
// OnSignal call stack, otherwise the worker is occupied and cannot execute the
// downstream tasks, causing a deadlock. Asynchronous consumption is the
// recommended pattern in both scheduler modes.
func (n *baseNode) askSignal(downstream []Node, value schema.Signal) *syncx.Collector[schema.Result] {
	if len(downstream) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return syncx.NewCollector[schema.Result](0)
	}
	collector := syncx.NewCollector[schema.Result](len(downstream))
	for _, next := range downstream {
		next.Input(&askSignal{Signal: value, collector: collector})
	}
	return collector
}

func (n *baseNode) SendPayload(value schema.Payload) {
	if len(n.table.payloads) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, nn := range n.table.payloads {
		nn.Input(value)
	}
}

func (n *baseNode) SendPayloadToPort(port int, value schema.Payload) {
	if port < 0 || port >= MaxPortCount || len(n.portTable[port].payloads) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, nn := range n.portTable[port].payloads {
		nn.Input(value)
	}
}

func (n *baseNode) SendAudio(value schema.Audio) {
	downstreamCount := len(n.table.audios)
	if downstreamCount == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for range downstreamCount {
		value.Retain()
	}
	for _, nn := range n.table.audios {
		nn.Input(value)
	}
}

func (n *baseNode) SendAudioToPort(port int, value schema.Audio) {
	if port < 0 || port >= MaxPortCount || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	nodes := n.portTable[port].audios
	downstreamCount := len(nodes)
	if downstreamCount == 0 {
		return
	}
	for range downstreamCount {
		value.Retain()
	}
	for _, nn := range nodes {
		nn.Input(value)
	}
}

func (n *baseNode) SendVideo(value schema.Video) {
	downstreamCount := len(n.table.videos)
	if downstreamCount == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for range downstreamCount {
		value.Retain()
	}
	for _, nn := range n.table.videos {
		nn.Input(value)
	}
}

func (n *baseNode) SendVideoToPort(port int, value schema.Video) {
	if port < 0 || port >= MaxPortCount || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	nodes := n.portTable[port].videos
	downstreamCount := len(nodes)
	if downstreamCount == 0 {
		return
	}
	for range downstreamCount {
		value.Retain()
	}
	for _, nn := range nodes {
		nn.Input(value)
	}
}

func (n *baseNode) Publish(mt protocol.PacketType, data []byte) {
	n.PublishFull(mt, protocol.EncodeRaw, data)
}

func (n *baseNode) PublishFull(mt protocol.PacketType, encode protocol.PacketEncode, data []byte) {
	if n.writer == nil {
		return
	}
	packet := protocol.AcquirePacket()
	packet.Type = mt
	packet.Encode = encode
	packet.SetPayload(data)
	n.writer.Write(packet)
}

func (n *baseNode) processControl(ctrl controlType) {
	switch ctrl {
	case controlPause:
		n.plugin.OnPause(n.ctx)
	case controlResume:
		n.plugin.OnResume(n.ctx, n)
	}
}

// processEvent dispatches a single event to the appropriate plugin handler.
//
// Signal and Payload no longer run under a 100ms deadline context: the deadline
// is only a soft SLA now, surfaced as a slow-handler warning log after the fact.
// This lets AskSignal downstreams perform real work before returning a Result
// without being canceled mid-flight.
//
// When the event is an *askSignal, the bare Signal is handed to OnSignal and the
// returned Result (if non-nil) is pushed into the collector. Done is always
// invoked via defer, so the asking side's collector closes even if the handler
// panics.
func (n *baseNode) processEvent(value schema.ReadOnly, _ bool) {
	if value == nil {
		return
	}

	start := time.Now()

	func() {
		defer func() {
			if ref, ok := value.(schema.RefCountable); ok {
				ref.Release()
			}
			if err := recover(); err != nil {
				slog.ErrorContext(n.ctx, "plugin panic recovered", "node", n.name, "error", err)
			}
			elapsed := time.Since(start)
			if elapsed > singleHandlerDeadline*time.Millisecond {
				slog.WarnContext(n.ctx, "handler execution slow",
					"node", n.name,
					"type", fmt.Sprintf("%T", value),
					"elapsed", elapsed,
					"limit", singleHandlerDeadline,
				)
			}
		}()
		switch v := value.(type) {
		case *askSignal:
			defer func() { _ = v.collector.Done() }()
			if result := n.plugin.OnSignal(n.ctx, n, v.Signal); result != nil {
				_ = v.collector.Put(result)
			}
		case schema.Signal:
			n.plugin.OnSignal(n.ctx, n, v)
		case schema.Payload:
			n.plugin.OnPayload(n.ctx, n, v)
		case schema.Audio:
			n.plugin.OnAudio(n.ctx, n, v)
		case schema.Video:
			n.plugin.OnVideo(n.ctx, n, v)
		}
	}()
}

func newNode(ctx context.Context, name string, plugin Plugin) *loopNode {
	return newLoopNode(ctx, name, plugin)
}
