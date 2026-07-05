package engine

import (
	"context"
	"errors"
	"log/slog"

	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

const (
	ctrlBufferSize    = 8
	signalBufferSize  = 12
	payloadBufferSize = 24
	audioBufferSize   = 64
	videoBufferSize   = 24
)

type loopNode struct {
	baseNode
	ctrlChan    chan controlType
	signalChan  chan schema.Signal
	payloadChan chan schema.Payload
	audioChan   chan schema.Audio
	videoChan   chan schema.Video
}

func newLoopNode(ctx context.Context, name string, plugin Plugin) *loopNode {
	return &loopNode{
		baseNode:    newBaseNode(ctx, name, plugin),
		ctrlChan:    make(chan controlType, ctrlBufferSize),
		signalChan:  make(chan schema.Signal, signalBufferSize),
		payloadChan: make(chan schema.Payload, payloadBufferSize),
		audioChan:   make(chan schema.Audio, audioBufferSize),
		videoChan:   make(chan schema.Video, videoBufferSize),
	}
}

func (n *loopNode) Start() error {
	if err := n.plugin.OnStart(n.ctx, n); err != nil {
		return err
	}
	n.running.Store(true)
	go n.readLoop()
	return nil
}

func (n *loopNode) Stop() {
	n.running.Store(false)
}

func (n *loopNode) Pause() {
	if !n.running.Load() {
		return
	}
	_ = syncx.SendWithContext(n.ctx, n.ctrlChan, controlPause)
}

func (n *loopNode) Resume() {
	if !n.running.Load() {
		return
	}
	_ = syncx.SendWithContext(n.ctx, n.ctrlChan, controlResume)
}

func (n *loopNode) Input(data schema.ReadOnly) {
	if n.ctx.Err() != nil || !n.running.Load() {
		if ref, ok := data.(schema.RefCountable); ok {
			ref.Release()
		}
		// A dropped askSignal must still release its collector slot,
		// otherwise the asking side blocks forever.
		finalizeDroppedAsk(data)
		return
	}
	switch v := data.(type) {
	case schema.Signal:
		if err := syncx.SendWithContext(n.ctx, n.signalChan, v); err != nil {
			finalizeDroppedAsk(v)
		}
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

func (n *loopNode) readLoop() {
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
			n.processEvent(event, true)
			continue
		case ctrl := <-n.ctrlChan:
			n.processControl(ctrl)
			continue
		default:
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

func (n *loopNode) drainChannels() {
	// Drain any pending signals first so that askSignals left in the buffer
	// release their collector slots and never leave an asker blocked.
	for {
		select {
		case sig := <-n.signalChan:
			finalizeDroppedAsk(sig)
			continue
		default:
		}
		break
	}
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
