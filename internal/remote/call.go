package remote

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
)

type call struct {
	ctx        context.Context
	id         string
	messageTyp pluginv1.RuntimeMessageType
	pluginName string
	instanceID string
	lease      time.Duration
	done       chan error
	renewCh    chan struct{}
	acked      atomic.Bool
	finished   atomic.Bool
}

func newCall(
	ctx context.Context,
	id string,
	messageTyp pluginv1.RuntimeMessageType,
	pluginName string,
	instanceID string,
	lease time.Duration,
) *call {
	return &call{
		ctx:        ctx,
		id:         id,
		messageTyp: messageTyp,
		pluginName: pluginName,
		instanceID: instanceID,
		lease:      lease,
		done:       make(chan error, 1),
		renewCh:    make(chan struct{}, 1),
	}
}

func (c *call) wait() error {
	timer := time.NewTimer(c.lease)
	defer timer.Stop()

	for {
		select {
		case err := <-c.done:
			return err
		case <-c.renewCh:
			resetTimer(timer, c.lease)
		case <-timer.C:
			err := fmt.Errorf("remote call lease expired: %s", c.id)
			c.finish(err)
			return err
		case <-c.ctx.Done():
			err := c.ctx.Err()
			c.finish(err)
			return err
		}
	}
}

func (c *call) renew() {
	if c.finished.Load() {
		return
	}
	if c.acked.CompareAndSwap(false, true) {
		slog.DebugContext(c.ctx, "remote call acknowledged",
			"plugin", c.pluginName,
			"type", c.messageTyp)
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

func resetTimer(timer *time.Timer, d time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}
