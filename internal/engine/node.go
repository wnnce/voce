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
)

const (
	singleHandlerDeadline = 100
)

type routeTable struct {
	signals  []Node
	payloads []Node
	audios   []Node
	videos   []Node
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

func (n *baseNode) processEvent(value schema.ReadOnly, useDeadline bool) {
	if value == nil {
		return
	}

	start := time.Now()
	var (
		currentCtx context.Context
		cancel     context.CancelFunc
	)

	if useDeadline {
		currentCtx, cancel = context.WithDeadline(n.ctx, start.Add(singleHandlerDeadline*time.Millisecond))
	} else {
		currentCtx = n.ctx
	}

	func() {
		defer func() {
			if ref, ok := value.(schema.RefCountable); ok {
				ref.Release()
			}
			if err := recover(); err != nil {
				slog.ErrorContext(n.ctx, "plugin panic recovered", "node", n.name, "error", err)
			}
			if cancel != nil {
				cancel()
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
		case schema.Signal:
			n.plugin.OnSignal(currentCtx, n, v)
		case schema.Payload:
			n.plugin.OnPayload(currentCtx, n, v)
		case schema.Audio:
			n.plugin.OnAudio(currentCtx, n, v)
		case schema.Video:
			n.plugin.OnVideo(currentCtx, n, v)
		}
	}()
}

func newNode(ctx context.Context, name string, plugin Plugin) *loopNode {
	return newLoopNode(ctx, name, plugin)
}
