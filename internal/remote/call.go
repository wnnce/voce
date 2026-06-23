package remote

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type call struct {
	ctx            context.Context
	id             string
	lease          time.Duration
	done           chan error
	renewCh        chan struct{}
	finished       atomic.Bool
	cancelCallback func() bool
	expireCallback func()
}

// configures one remote call.
type callOption func(*call)

// sets the callback invoked after the call context is canceled.
func withCancelCallback(callback func() bool) callOption {
	return func(c *call) {
		c.cancelCallback = callback
	}
}

// sets the callback invoked after the call lease expires.
func withExpireCallback(callback func()) callOption {
	return func(c *call) {
		c.expireCallback = callback
	}
}

func newCall(
	ctx context.Context,
	id string,
	lease time.Duration,
	options ...callOption,
) *call {
	c := &call{
		ctx:     ctx,
		id:      id,
		lease:   lease,
		done:    make(chan error, 1),
		renewCh: make(chan struct{}, 1),
	}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c *call) wait() error {
	timer := time.NewTimer(c.lease)
	defer timer.Stop()
	ctxDone := c.ctx.Done()
	for {
		select {
		case err := <-c.done:
			return err
		case <-c.renewCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(c.lease)
		case <-timer.C:
			err := fmt.Errorf("remote call lease expired: %s", c.id)
			if !c.finished.CompareAndSwap(false, true) {
				return nil
			}
			if c.expireCallback != nil {
				c.expireCallback()
			}
			return err
		case <-ctxDone:
			ctxDone = nil
			// keep waiting
			if c.cancelCallback != nil && !c.cancelCallback() {
				continue
			}
			if !c.finished.CompareAndSwap(false, true) {
				return nil
			}
			return c.ctx.Err()
		}
	}
}

func (c *call) renew() {
	if c.finished.Load() {
		return
	}
	select {
	case c.renewCh <- struct{}{}:
	default:
	}
}

func (c *call) finish(err error) {
	if !c.finished.CompareAndSwap(false, true) {
		return
	}
	c.done <- err
}
