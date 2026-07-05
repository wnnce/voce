package syncx

import (
	"errors"
	"sync/atomic"
)

var (
	ErrCollectorClosed   = errors.New("collector closed")
	ErrCollectorCanceled = errors.New("collector canceled")
	ErrNegativePending   = errors.New("collector pending count became negative")
)

type Collector[T any] struct {
	ch       chan T
	done     chan struct{}
	pending  atomic.Int64
	closed   atomic.Bool
	canceled atomic.Bool
}

func NewCollector[T any](count int) *Collector[T] {
	if count < 0 {
		count = 0
	}
	c := &Collector[T]{
		ch:   make(chan T, count),
		done: make(chan struct{}),
	}
	c.pending.Store(int64(count))
	if count == 0 {
		c.tryClose()
	}
	return c
}

func (c *Collector[T]) Chan() <-chan T {
	return c.ch
}

func (c *Collector[T]) Done() error {
	next := c.pending.Add(-1)
	switch {
	case next > 0:
		return nil
	case next == 0:
		c.tryClose()
		return nil
	default:
		c.pending.Add(1)
		return ErrNegativePending
	}
}

func (c *Collector[T]) Put(value T) error {
	if c.canceled.Load() {
		return ErrCollectorCanceled
	}
	if c.closed.Load() {
		return ErrCollectorClosed
	}
	select {
	case <-c.done:
		if c.canceled.Load() {
			return ErrCollectorCanceled
		}
		return ErrCollectorClosed
	case c.ch <- value:
		return nil
	}
}

func (c *Collector[T]) Cancel() bool {
	c.canceled.Store(true)
	return c.tryClose()
}

func (c *Collector[T]) Wait() {
	<-c.done
}

func (c *Collector[T]) Pending() int64 {
	return c.pending.Load()
}

func (c *Collector[T]) Closed() bool {
	return c.closed.Load()
}

func (c *Collector[T]) Canceled() bool {
	return c.canceled.Load()
}

func (c *Collector[T]) tryClose() bool {
	if !c.closed.CompareAndSwap(false, true) {
		return false
	}
	close(c.ch)
	close(c.done)
	return true
}
