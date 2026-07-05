package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

// askMockPlugin is a test plugin whose OnSignal returns a named Result. It lets
// tests exercise the AskSignal collection path with configurable behavior.
type askMockPlugin struct {
	BuiltinPlugin
	resultName    string        // when non-empty, OnSignal returns a Result with this name
	delay         time.Duration // simulates a downstream doing real work before returning
	forward       bool          // re-emits the bare signal downstream via SendSignal
	panicOnSignal bool          // makes OnSignal panic, to verify Done still fires
}

func (p *askMockPlugin) OnSignal(_ context.Context, flow Flow, signal schema.Signal) schema.Result {
	if p.panicOnSignal {
		panic("boom")
	}
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.forward {
		flow.SendSignal(signal)
	}
	if p.resultName == "" {
		return nil
	}
	return schema.NewResultBuilder(p.resultName, schema.ResultStatusOK).Build()
}

// collectResultNames drains a collector's channel into a slice of result names.
func collectResultNames(c *syncx.Collector[schema.Result]) ([]string, error) {
	names := make([]string, 0)
	deadline := time.After(time.Second)
	ch := c.Chan()
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				return names, nil
			}
			names = append(names, r.Name())
		case <-deadline:
			return names, fmt.Errorf("collector did not close within %v (got %v so far)", time.Second, names)
		}
	}
}

func TestAskSignal(t *testing.T) {
	// CollectsDirectDownstreamResults verifies that AskSignal gathers one Result
	// per direct downstream node.
	t.Run("CollectsDirectDownstreamResults", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		for _, name := range []string{"a", "b", "c"} {
			recv := newNode(ctx, "recv-"+name, &askMockPlugin{resultName: "res-" + name})
			require.NoError(t, recv.Start())
			sender.addNextNode(EventSignal, recv)
		}

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		require.NotNil(t, collector)

		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"res-a", "res-b", "res-c"}, names)
	})

	// NilResultsAreSkipped verifies that downstreams returning nil still close the
	// collector (via Done) but contribute no value.
	t.Run("NilResultsAreSkipped", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		withRes := newNode(ctx, "with", &askMockPlugin{resultName: "only"})
		require.NoError(t, withRes.Start())
		sender.addNextNode(EventSignal, withRes)

		for range 2 {
			nilRecv := newNode(ctx, "nil", &askMockPlugin{})
			require.NoError(t, nilRecv.Start())
			sender.addNextNode(EventSignal, nilRecv)
		}

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.Equal(t, []string{"only"}, names)
	})

	// NoDownstreamClosesImmediately verifies that asking with no signal
	// downstreams returns an already-closed empty collector.
	t.Run("NoDownstreamClosesImmediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		require.NotNil(t, collector)
		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	// ForwardingDoesNotPropagateCollector verifies that when a direct downstream
	// forwards the signal further via SendSignal, the deeper node's result is NOT
	// collected: the collector only gathers direct downstream results.
	t.Run("ForwardingDoesNotPropagateCollector", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		deep := newNode(ctx, "deep", &askMockPlugin{resultName: "deep"})
		require.NoError(t, deep.Start())

		direct := newNode(ctx, "direct", &askMockPlugin{resultName: "direct", forward: true})
		require.NoError(t, direct.Start())
		direct.addNextNode(EventSignal, deep)

		sender.addNextNode(EventSignal, direct)

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.Equal(t, []string{"direct"}, names)
	})

	// ExceedsFormerDeadline verifies that a downstream taking longer than the old
	// 100ms handler deadline still returns its Result (the deadline is now only a
	// soft warning, not a cancellation).
	t.Run("ExceedsFormerDeadline", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		slow := newNode(ctx, "slow", &askMockPlugin{resultName: "slow", delay: 150 * time.Millisecond})
		require.NoError(t, slow.Start())
		sender.addNextNode(EventSignal, slow)

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.Equal(t, []string{"slow"}, names)
	})

	// PanicStillClosesCollector verifies that a panicking downstream handler does
	// not leave the asker blocked: Done fires via defer.
	t.Run("PanicStillClosesCollector", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		good := newNode(ctx, "good", &askMockPlugin{resultName: "good"})
		require.NoError(t, good.Start())
		sender.addNextNode(EventSignal, good)

		bad := newNode(ctx, "bad", &askMockPlugin{panicOnSignal: true})
		require.NoError(t, bad.Start())
		sender.addNextNode(EventSignal, bad)

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		names, err := collectResultNames(collector)
		require.NoError(t, err)
		assert.Equal(t, []string{"good"}, names)
	})

	// StoppedDownstreamDoesNotBlock verifies that when a downstream node is stopped
	// (drops inputs), finalizeDroppedAsk still releases its collector slot so the
	// asker's collector closes.
	t.Run("StoppedDownstreamDoesNotBlock", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sender := newNode(ctx, "sender", &BuiltinPlugin{})
		require.NoError(t, sender.Start())

		live := newNode(ctx, "live", &askMockPlugin{resultName: "live"})
		require.NoError(t, live.Start())
		sender.addNextNode(EventSignal, live)

		stopped := newNode(ctx, "stopped", &askMockPlugin{resultName: "stopped"})
		require.NoError(t, stopped.Start())
		stopped.Stop()
		sender.addNextNode(EventSignal, stopped)

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())

		done := make(chan collectResult, 1)
		go func() {
			names, err := collectResultNames(collector)
			done <- collectResult{names: names, err: err}
		}()

		select {
		case result := <-done:
			require.NoError(t, result.err)
			assert.Contains(t, result.names, "live")
			assert.NotContains(t, result.names, "stopped")
		case <-time.After(2 * time.Second):
			t.Fatal("collector blocked when a downstream was stopped")
		}
	})

	// SchedulerModeConsistent verifies AskSignal works under the worker-pool
	// scheduler too, provided the collector is consumed asynchronously.
	t.Run("SchedulerModeConsistent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		s := NewScheduler(ctx, 2, 32)
		defer s.Stop()

		sender := newSchedulerNode(ctx, "sender", &BuiltinPlugin{}, s)
		require.NoError(t, sender.Start())

		for _, name := range []string{"x", "y"} {
			recv := newSchedulerNode(ctx, "recv-"+name, &askMockPlugin{resultName: "r-" + name}, s)
			require.NoError(t, recv.Start())
			sender.addNextNode(EventSignal, recv)
		}

		collector := sender.AskSignal(schema.NewSignal("ask").ReadOnly())
		require.NotNil(t, collector)

		// Consume asynchronously (required in worker-pool mode).
		done := make(chan collectResult, 1)
		go func() {
			names, err := collectResultNames(collector)
			done <- collectResult{names: names, err: err}
		}()

		select {
		case result := <-done:
			require.NoError(t, result.err)
			assert.ElementsMatch(t, []string{"r-x", "r-y"}, result.names)
		case <-time.After(2 * time.Second):
			t.Fatal("worker-pool AskSignal did not complete")
		}
	})
}

type collectResult struct {
	names []string
	err   error
}
