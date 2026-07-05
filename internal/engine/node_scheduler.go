package engine

import (
	"context"

	"github.com/wnnce/voce/internal/schema"
)

type schedulerNode struct {
	baseNode
	scheduler *Scheduler
	workerIdx int
}

func newSchedulerNode(ctx context.Context, name string, plugin Plugin, s *Scheduler) *schedulerNode {
	workerIdx := s.selector(name)
	return &schedulerNode{
		baseNode:  newBaseNode(ctx, name, plugin),
		scheduler: s,
		workerIdx: workerIdx,
	}
}

func (n *schedulerNode) Start() error {
	if err := n.plugin.OnStart(n.ctx, n); err != nil {
		return err
	}
	n.running.Store(true)
	return nil
}

func (n *schedulerNode) Stop() {
	if n.running.Swap(false) {
		n.plugin.OnStop()
	}
}

func (n *schedulerNode) Pause() {
	if !n.running.Load() {
		return
	}
	n.scheduler.SubmitControlToWorker(n.workerIdx, n, controlPause)
}

func (n *schedulerNode) Resume() {
	if !n.running.Load() {
		return
	}
	n.scheduler.SubmitControlToWorker(n.workerIdx, n, controlResume)
}

func (n *schedulerNode) Input(data schema.ReadOnly) {
	if n.ctx.Err() != nil || !n.running.Load() {
		if ref, ok := data.(schema.RefCountable); ok {
			ref.Release()
		}
		// A dropped askSignal must still release its collector slot,
		// otherwise the asking side blocks forever.
		finalizeDroppedAsk(data)
		return
	}
	n.scheduler.SubmitToWorker(n.workerIdx, n, data)
}
