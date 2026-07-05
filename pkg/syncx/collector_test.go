package syncx

import (
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorAddDoneAndCollect(t *testing.T) {
	t.Run("collects values and closes on zero pending", func(t *testing.T) {
		collector := NewCollector[int](2)

		require.NoError(t, collector.Put(1))
		require.NoError(t, collector.Done())
		require.NoError(t, collector.Put(2))
		require.NoError(t, collector.Done())

		var got []int
		for value := range collector.Chan() {
			got = append(got, value)
		}

		assert.Equal(t, []int{1, 2}, got)
		assert.True(t, collector.Closed())
		assert.False(t, collector.Canceled())
		assert.EqualValues(t, 0, collector.Pending())
	})

	t.Run("done without matching add returns error", func(t *testing.T) {
		collector := NewCollector[int](0)
		require.ErrorIs(t, collector.Done(), ErrNegativePending)
		assert.True(t, collector.Closed())
	})
}

func TestCollectorWait(t *testing.T) {
	t.Run("wait unblocks after pending reaches zero", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			collector := NewCollector[string](1)

			waited := make(chan struct{})
			go func() {
				collector.Wait()
				close(waited)
			}()

			synctest.Wait()
			select {
			case <-waited:
				t.Fatal("wait returned before collector finished")
			default:
			}

			require.NoError(t, collector.Put("done"))
			require.NoError(t, collector.Done())

			synctest.Wait()
			select {
			case <-waited:
			default:
				t.Fatal("wait did not return after collector finished")
			}
		})
	})
}

func TestCollectorCancel(t *testing.T) {
	t.Run("cancel closes collector and rejects future put", func(t *testing.T) {
		collector := NewCollector[int](1)

		assert.True(t, collector.Cancel())
		assert.True(t, collector.Closed())
		assert.True(t, collector.Canceled())
		require.ErrorIs(t, collector.Put(1), ErrCollectorCanceled)
	})

	t.Run("wait returns after cancel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			collector := NewCollector[int](1)

			done := make(chan struct{})
			go func() {
				collector.Wait()
				close(done)
			}()

			collector.Cancel()
			synctest.Wait()

			select {
			case <-done:
			default:
				t.Fatal("wait did not return after cancel")
			}
		})
	})
}

func TestCollectorCloseAndCollectState(t *testing.T) {
	t.Run("put after normal close returns closed error", func(t *testing.T) {
		collector := NewCollector[int](1)
		require.NoError(t, collector.Done())

		require.ErrorIs(t, collector.Put(7), ErrCollectorClosed)
	})

	t.Run("second cancel returns false", func(t *testing.T) {
		collector := NewCollector[int](1)
		assert.True(t, collector.Cancel())
		assert.False(t, collector.Cancel())
	})

	t.Run("zero count starts closed", func(t *testing.T) {
		collector := NewCollector[int](0)
		assert.True(t, collector.Closed())
		assert.EqualValues(t, 0, collector.Pending())
	})
}

func TestCollectorBusinessScenarios(t *testing.T) {
	t.Run("fanout ask signal collects results from multiple downstream handlers", func(t *testing.T) {
		collector := NewCollector[string](3)

		require.NoError(t, collector.Put("node-a:ok"))
		require.NoError(t, collector.Done())
		require.NoError(t, collector.Put("node-b:ok"))
		require.NoError(t, collector.Done())
		require.NoError(t, collector.Put("node-c:ok"))
		require.NoError(t, collector.Done())

		var got []string
		for value := range collector.Chan() {
			got = append(got, value)
		}

		assert.Equal(t, []string{
			"node-a:ok",
			"node-b:ok",
			"node-c:ok",
		}, got)
	})

	t.Run("downstream handler may complete without producing a result", func(t *testing.T) {
		collector := NewCollector[string](3)

		require.NoError(t, collector.Put("node-a:ok"))
		require.NoError(t, collector.Done())
		require.NoError(t, collector.Done())
		require.NoError(t, collector.Put("node-c:ok"))
		require.NoError(t, collector.Done())

		var got []string
		for value := range collector.Chan() {
			got = append(got, value)
		}

		assert.Equal(t, []string{
			"node-a:ok",
			"node-c:ok",
		}, got)
	})

	t.Run("consumer may wait first and drain results after completion", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			collector := NewCollector[int](2)
			errs := make(chan error, 4)

			go func() {
				errs <- collector.Put(10)
				errs <- collector.Done()
				errs <- collector.Put(20)
				errs <- collector.Done()
			}()

			collector.Wait()
			synctest.Wait()

			for range 4 {
				require.NoError(t, <-errs)
			}

			var got []int
			for value := range collector.Chan() {
				got = append(got, value)
			}

			assert.Equal(t, []int{10, 20}, got)
		})
	})

	t.Run("cancel stops late results from slow downstream workers", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			collector := NewCollector[string](2)
			firstErrs := make(chan error, 2)
			lateErrs := make(chan error, 2)

			// canceled gates the slow worker: it must not attempt its Put
			// until the collector has actually been canceled, otherwise the
			// Put would simply land in the still-open buffer.
			canceled := make(chan struct{})
			go func() {
				firstErrs <- collector.Put("node-a:partial")
				firstErrs <- collector.Done()
			}()

			go func() {
				<-canceled
				lateErrs <- collector.Put("node-b:late")
				lateErrs <- collector.Done()
			}()

			synctest.Wait()
			for range 2 {
				require.NoError(t, <-firstErrs)
			}

			var got []string
			got = append(got, <-collector.Chan())
			assert.True(t, collector.Cancel())
			close(canceled)

			collector.Wait()
			synctest.Wait()

			require.ErrorIs(t, <-lateErrs, ErrCollectorCanceled)
			require.NoError(t, <-lateErrs)

			for value := range collector.Chan() {
				got = append(got, value)
			}

			assert.Equal(t, []string{"node-a:partial"}, got)
		})
	})
}
