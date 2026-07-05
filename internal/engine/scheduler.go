package engine

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"

	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

type SchedulerMode string

const (
	SchedulerModeThreadPerNode SchedulerMode = "thread-per-node"
	SchedulerModeWorkerPool    SchedulerMode = "worker-pool"
)

type Task struct {
	node    Node
	event   schema.ReadOnly
	control controlType
}

type worker struct {
	ctx              context.Context
	highPriorityCh   chan Task
	normalPriorityCh chan Task
}

func newWorker(ctx context.Context, queueSize int) *worker {
	return &worker{
		ctx:              ctx,
		highPriorityCh:   make(chan Task, queueSize),
		normalPriorityCh: make(chan Task, queueSize),
	}
}

func (w *worker) readLoop() {
	defer func() {
		w.drain()
	}()
	for {
		select {
		case <-w.ctx.Done():
			return
		case task := <-w.highPriorityCh:
			w.execute(task)
			continue
		default:
			select {
			case <-w.ctx.Done():
				return
			case task := <-w.highPriorityCh:
				w.execute(task)
			case task := <-w.normalPriorityCh:
				w.execute(task)
			}
		}
	}
}

func (w *worker) execute(task Task) {
	if task.control != 0 {
		task.node.processControl(task.control)
		return
	}

	// We determine if we need a deadline.
	// useDeadline is true for control/metadata events (Signal, Payload)
	// and false for media streaming events (Audio, Video).
	var useDeadline bool
	switch task.event.(type) {
	case schema.Signal, schema.Payload:
		useDeadline = true
	}

	task.node.processEvent(task.event, useDeadline)
}

func (w *worker) drain() {
	for {
		var task Task
		select {
		case task = <-w.highPriorityCh:
		case task = <-w.normalPriorityCh:
		default:
			return
		}
		if ref, ok := task.event.(schema.RefCountable); ok {
			ref.Release()
		}
		// A drained askSignal must still release its collector slot,
		// otherwise the asking side blocks forever.
		finalizeDroppedAsk(task.event)
	}
}

type Scheduler struct {
	ctx      context.Context
	cancel   context.CancelFunc
	workers  []*worker
	selector func(nodeName string) int
}

func NewScheduler(parentCtx context.Context, workerCount int, queueSize int) *Scheduler {
	ctx, cancel := context.WithCancel(parentCtx)
	if workerCount <= 0 {
		workerCount = 1
	}
	if queueSize <= 0 {
		queueSize = 100
	}
	workers := make([]*worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers[i] = newWorker(ctx, queueSize)
		go workers[i].readLoop()
	}
	return &Scheduler{
		ctx:     ctx,
		cancel:  cancel,
		workers: workers,
		selector: func(nodeName string) int {
			h := fnv.New32a()
			_, _ = h.Write([]byte(nodeName))
			return int(h.Sum32()) % workerCount
		},
	}
}

func (s *Scheduler) Stop() {
	s.cancel()
}

func (s *Scheduler) Submit(n Node, data schema.ReadOnly) {
	workerIdx := s.selector(n.Name())
	s.SubmitToWorker(workerIdx, n, data)
}

func (s *Scheduler) SubmitToWorker(workerIdx int, n Node, data schema.ReadOnly) {
	w := s.workers[workerIdx]
	task := Task{node: n, event: data}

	if _, ok := data.(schema.Signal); ok {
		if err := syncx.SendWithContext(s.ctx, w.highPriorityCh, task); err != nil {
			finalizeDroppedAsk(data)
		}
		return
	}
	if _, ok := data.(schema.Payload); ok {
		_ = syncx.SendWithContext(s.ctx, w.normalPriorityCh, task)
		return
	}
	if err := syncx.SendWithNonBlocking(s.ctx, w.normalPriorityCh, task); err != nil {
		if ref, ok := data.(schema.RefCountable); ok {
			ref.Release()
		}
		if errors.Is(err, syncx.ErrSendBlocked) {
			slog.ErrorContext(n.Context(), "media packet dropped by scheduler", "node", n.Name(), "type", fmt.Sprintf("%T", data))
		}
	}
}

func (s *Scheduler) SubmitControl(n Node, ctrl controlType) {
	workerIdx := s.selector(n.Name())
	s.SubmitControlToWorker(workerIdx, n, ctrl)
}

func (s *Scheduler) SubmitControlToWorker(workerIdx int, n Node, ctrl controlType) {
	w := s.workers[workerIdx]

	task := Task{node: n, control: ctrl}

	if err := syncx.SendWithContext(s.ctx, w.highPriorityCh, task); err != nil {
		slog.ErrorContext(n.Context(), "control packet dropped by scheduler", "node", n.Name(), "control", ctrl)
	}
}
