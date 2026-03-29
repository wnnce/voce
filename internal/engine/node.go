package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

const (
	ctrlBufferSize        = 8
	signalBufferSize      = 12
	payloadBufferSize     = 24
	audioBufferSize       = 64
	videoBufferSize       = 24
	singleHandlerDeadline = 100
)

type routeTable struct {
	signals  []*node
	payloads []*node
	audios   []*node
	videos   []*node
}

// node is the active runtime instance of a plugin in the workflow graph.
// It manages its own event loop and buffers for various event types.
type node struct {
	ctx         context.Context
	plugin      Plugin
	ctrlChan    chan controlType
	signalChan  chan schema.Signal
	payloadChan chan schema.Payload
	audioChan   chan schema.Audio
	videoChan   chan schema.Video
	name        string
	table       routeTable
	portTable   [MaxPortCount]routeTable
	running     atomic.Bool
	writer      SocketWriter
}

func newNode(ctx context.Context, name string, plugin Plugin) *node {
	return &node{
		ctx:         ctx,
		plugin:      plugin,
		name:        name,
		ctrlChan:    make(chan controlType, ctrlBufferSize),
		signalChan:  make(chan schema.Signal, signalBufferSize),
		payloadChan: make(chan schema.Payload, payloadBufferSize),
		audioChan:   make(chan schema.Audio, audioBufferSize),
		videoChan:   make(chan schema.Video, videoBufferSize),
	}
}

func (n *node) setSocketWriter(writer SocketWriter) {
	n.writer = writer
}

func (n *node) start() error {
	if err := n.plugin.OnStart(n.ctx, n); err != nil {
		return err
	}
	n.running.Store(true)
	go n.readLoop()
	return nil
}

func (n *node) ready() {
	if !n.running.Load() {
		return
	}
	n.plugin.OnReady(n.ctx, n)
}

func (n *node) stop() {
	n.running.Store(false)
}

func (n *node) pause() {
	if !n.running.Load() {
		return
	}
	_ = syncx.SendWithContext(n.ctx, n.ctrlChan, controlPause)
}

func (n *node) resume() {
	if !n.running.Load() {
		return
	}
	_ = syncx.SendWithContext(n.ctx, n.ctrlChan, controlResume)
}

func (n *node) addNextNode(event EventType, nextNode *node) {
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

func (n *node) addNextPortNode(event EventType, nextNode *node, port int) {
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

func (n *node) readLoop() {
	defer func() {
		n.running.Store(false)
		n.plugin.OnStop()
		n.drainChannels()
	}()
	for {
		if n.ctx.Err() != nil || !n.running.Load() {
			return
		}
		var (
			event       schema.ReadOnly
			useDeadline bool
		)
		select {
		case <-n.ctx.Done():
			return
		case event = <-n.signalChan:
			// High-priority: signals (control, interruption) always processed first.
			n.processEvent(event, true)
			continue
		case ctrl := <-n.ctrlChan:
			n.processControl(ctrl)
			continue
		default:
			// Normal flow: prioritize control signals then media channels.
			select {
			case <-n.ctx.Done():
				return
			case ctrl := <-n.ctrlChan:
				n.processControl(ctrl)
			case event = <-n.signalChan:
				useDeadline = true
			case event = <-n.payloadChan:
				useDeadline = true
			case event = <-n.audioChan:
				useDeadline = false
			case event = <-n.videoChan:
				useDeadline = false
			}
		}

		n.processEvent(event, useDeadline)
	}
}

func (n *node) drainChannels() {
	for {
		var event schema.ReadOnly
		select {
		case event = <-n.audioChan:
		case event = <-n.videoChan:
		default:
			return
		}
		if event == nil {
			continue
		}
		if ref, ok := event.(schema.RefCountable); ok {
			ref.Release()
		}
	}
}

func (n *node) processControl(ctrl controlType) {
	switch ctrl {
	case controlPause:
		n.plugin.OnPause(n.ctx)
	case controlResume:
		n.plugin.OnResume(n.ctx, n)
	}
}

func (n *node) processEvent(value schema.ReadOnly, useDeadline bool) {
	if value == nil {
		return
	}

	start := time.Now()
	var (
		currentCtx context.Context
		cancel     context.CancelFunc
	)

	if useDeadline {
		// Enforce a processing timeout for control/data events to ensure node responsiveness.
		currentCtx, cancel = context.WithDeadline(n.ctx, start.Add(singleHandlerDeadline*time.Millisecond))
	} else {
		currentCtx = n.ctx
	}

	func() {
		defer func() {
			if ref, ok := value.(schema.RefCountable); ok {
				// Automatic memory management: release back to the object pool.
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

func (n *node) Input(data schema.ReadOnly) {
	if n.ctx.Err() != nil || !n.running.Load() {
		if ref, ok := data.(schema.RefCountable); ok {
			ref.Release()
		}
		return
	}
	switch v := data.(type) {
	case schema.Signal:
		_ = syncx.SendWithContext(n.ctx, n.signalChan, v)
	case schema.Payload:
		_ = syncx.SendWithContext(n.ctx, n.payloadChan, v)
	case schema.Audio:
		if err := syncx.SendWithNonBlocking(n.ctx, n.audioChan, v); err != nil {
			v.Release()
			if errors.Is(err, syncx.ErrSendBlocked) {
				slog.ErrorContext(n.ctx, "audio dropped", "node", n.name)
			}
		}
	case schema.Video:
		if err := syncx.SendWithNonBlocking(n.ctx, n.videoChan, v); err != nil {
			v.Release()
			if errors.Is(err, syncx.ErrSendBlocked) {
				slog.ErrorContext(n.ctx, "video dropped", "node", n.name)
			}
		}
	}
}

func (n *node) Context() context.Context {
	return n.ctx
}

func (n *node) SendSignal(value schema.Signal) {
	if len(n.table.signals) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, next := range n.table.signals {
		next.Input(value)
	}
}

func (n *node) SendSignalToPort(port int, value schema.Signal) {
	if port < 0 || port >= MaxPortCount || len(n.portTable[port].signals) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, next := range n.portTable[port].signals {
		next.Input(value)
	}
}

func (n *node) SendPayload(value schema.Payload) {
	if len(n.table.payloads) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, nn := range n.table.payloads {
		nn.Input(value)
	}
}

func (n *node) SendPayloadToPort(port int, value schema.Payload) {
	if port < 0 || port >= MaxPortCount || len(n.portTable[port].payloads) == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for _, nn := range n.portTable[port].payloads {
		nn.Input(value)
	}
}

func (n *node) SendAudio(value schema.Audio) {
	downstreamCount := len(n.table.audios)
	if downstreamCount == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for i := 0; i < downstreamCount; i++ {
		value.Retain()
	}
	for _, nn := range n.table.audios {
		nn.Input(value)
	}
}

func (n *node) SendAudioToPort(port int, value schema.Audio) {
	if port < 0 || port >= MaxPortCount || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	nodes := n.portTable[port].audios
	downstreamCount := len(nodes)
	if downstreamCount == 0 {
		return
	}
	for i := 0; i < downstreamCount; i++ {
		value.Retain()
	}
	for _, nn := range nodes {
		nn.Input(value)
	}
}

func (n *node) SendVideo(value schema.Video) {
	downstreamCount := len(n.table.videos)
	if downstreamCount == 0 || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	for i := 0; i < downstreamCount; i++ {
		value.Retain()
	}
	for _, nn := range n.table.videos {
		nn.Input(value)
	}
}

func (n *node) SendVideoToPort(port int, value schema.Video) {
	if port < 0 || port >= MaxPortCount || n.ctx.Err() != nil || !n.running.Load() {
		return
	}
	nodes := n.portTable[port].videos
	downstreamCount := len(nodes)
	if downstreamCount == 0 {
		return
	}
	for i := 0; i < downstreamCount; i++ {
		value.Retain()
	}
	for _, nn := range nodes {
		nn.Input(value)
	}
}

func (n *node) Publish(mt protocol.PacketType, data []byte) {
	n.PublishFull(mt, protocol.EncodeRaw, data)
}

func (n *node) PublishFull(mt protocol.PacketType, encode protocol.PacketEncode, data []byte) {
	if n.writer == nil {
		return
	}
	packet := protocol.AcquirePacket()
	packet.Type = mt
	packet.Encode = encode
	packet.SetPayload(data)
	n.writer.Write(packet)
}
