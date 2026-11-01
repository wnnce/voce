package buf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChunkQueue_WriteReadAcrossBlocks(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)

	n, err := q.Write([]byte("abcdef"))
	require.NoError(t, err)
	require.Equal(t, 6, n)
	require.Equal(t, 6, q.Len())

	first := make([]byte, 2)
	n, err = q.Read(first)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []byte("ab"), first)

	rest := make([]byte, 4)
	n, err = q.Read(rest)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte("cdef"), rest)
	require.Zero(t, q.Len())

	_, err = q.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func TestChunkQueue_WriteAfterPartialRead(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)
	_, err := q.Write([]byte("abcdef"))
	require.NoError(t, err)

	prefix := make([]byte, 2)
	_, err = q.Read(prefix)
	require.NoError(t, err)
	require.Equal(t, []byte("ab"), prefix)

	_, err = q.Write([]byte("ghij"))
	require.NoError(t, err)
	remaining := make([]byte, 8)
	_, err = q.Read(remaining)
	require.NoError(t, err)
	require.Equal(t, []byte("cdefghij"), remaining)
}

func TestChunkQueue_WriteCopiesInput(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)
	source := []byte("abcdef")
	_, err := q.Write(source)
	require.NoError(t, err)
	for i := range source {
		source[i] = 'x'
	}

	got := make([]byte, len(source))
	_, err = q.Read(got)
	require.NoError(t, err)
	require.Equal(t, []byte("abcdef"), got)
}

func TestChunkQueue_ReadKeepsDestinationAfterRecycle(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)
	_, err := q.Write([]byte("abcdefgh"))
	require.NoError(t, err)

	got := make([]byte, 8)
	_, err = q.Read(got)
	require.NoError(t, err)
	q.Reset()
	require.Equal(t, []byte("abcdefgh"), got)
}

func TestChunkQueue_BoundedPartialWrite(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 5)

	n, err := q.Write([]byte("abcdefg"))
	require.Equal(t, 5, n)
	require.ErrorIs(t, err, ErrChunkQueueFull)
	require.Equal(t, 5, q.Len())

	data := make([]byte, 5)
	n, err = q.Read(data)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, []byte("abcde"), data)

	n, err = q.Write([]byte("fg"))
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestChunkQueue_BoundedExactCapacityAndRecovery(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 8)

	n, err := q.Write([]byte("12345678"))
	require.NoError(t, err)
	require.Equal(t, 8, n)
	n, err = q.Write([]byte("9"))
	require.ErrorIs(t, err, ErrChunkQueueFull)
	require.Zero(t, n)

	data := make([]byte, 4)
	_, err = q.Read(data)
	require.NoError(t, err)
	require.Equal(t, []byte("1234"), data)

	n, err = q.Write([]byte("90"))
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 6, q.Len())
}

func TestChunkQueue_EmptyOperations(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)

	n, err := q.Write(nil)
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = q.Read(nil)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Zero(t, q.Len())
}

func TestChunkQueue_ResetReusesQueue(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(4), 0)
	_, err := q.Write([]byte("abcdef"))
	require.NoError(t, err)

	q.Reset()
	require.Zero(t, q.Len())
	_, err = q.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)

	_, err = q.Write([]byte("xy"))
	require.NoError(t, err)
	data := make([]byte, 2)
	_, err = q.Read(data)
	require.NoError(t, err)
	require.Equal(t, []byte("xy"), data)
}

func TestChunkQueue_ConcurrentWriteRead(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(7), 0)
	const (
		writes    = 1000
		writeSize = 13
	)

	input := make([]byte, writes*writeSize)
	for i := 0; i < writes; i++ {
		for j := 0; j < writeSize; j++ {
			input[i*writeSize+j] = byte(i % 251)
		}
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; i < writes; i++ {
			for {
				n, err := q.Write(input[i*writeSize : (i+1)*writeSize])
				if err == nil {
					if n != writeSize {
						panic("short write")
					}
					break
				}
				panic(err)
			}
		}
	}()

	readDone := make(chan []byte, 1)
	go func() {
		var output bytes.Buffer
		chunk := make([]byte, 11)
		for output.Len() < len(input) {
			n, err := q.Read(chunk)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					panic(err)
				}
				runtime.Gosched()
				continue
			}
			_, _ = output.Write(chunk[:n])
		}
		readDone <- output.Bytes()
	}()

	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("writer did not finish")
	}
	select {
	case output := <-readDone:
		require.Equal(t, input, output)
	case <-time.After(time.Second):
		t.Fatal("reader did not finish")
	}
}

func TestChunkQueue_SharedPool(t *testing.T) {
	pool := NewChunkPool(4)
	first := NewChunkQueue(pool, 0)
	second := NewChunkQueue(pool, 0)

	_, err := first.Write([]byte("abcdefgh"))
	require.NoError(t, err)
	first.Reset()

	_, err = second.Write([]byte("ijkl"))
	require.NoError(t, err)
	data := make([]byte, 4)
	_, err = second.Read(data)
	require.NoError(t, err)
	require.Equal(t, []byte("ijkl"), data)
}

func TestChunkQueue_ConcurrentWritersKeepWritesContiguous(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(5), 0)
	const (
		writers     = 4
		writesEach  = 100
		recordBytes = 9
	)

	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record := bytes.Repeat([]byte{byte(writer)}, recordBytes)
			for range writesEach {
				if n, err := q.Write(record); err != nil || n != recordBytes {
					panic("unexpected concurrent write result")
				}
			}
		}()
	}
	wg.Wait()

	all := make([]byte, writers*writesEach*recordBytes)
	n, err := q.Read(all)
	require.NoError(t, err)
	require.Equal(t, len(all), n)
	counts := make([]int, writers)
	for offset := 0; offset < len(all); offset += recordBytes {
		record := all[offset : offset+recordBytes]
		for _, value := range record {
			require.Equal(t, record[0], value, "a Write must remain contiguous")
		}
		require.Less(t, int(record[0]), writers)
		counts[record[0]]++
	}
	for _, count := range counts {
		require.Equal(t, writesEach, count)
	}
}

func TestChunkQueue_ConcurrentBoundedWritersRespectLimit(t *testing.T) {
	const limit = 64
	q := NewChunkQueue(NewChunkPool(7), limit)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = q.Write(bytes.Repeat([]byte{'x'}, 17))
		}()
	}
	wg.Wait()
	require.LessOrEqual(t, q.Len(), limit)
}

func TestChunkQueue_ConcurrentReadersConsumeEachByteOnce(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(13), 0)
	const total = 100_000
	input := bytes.Repeat([]byte{0xa5}, total)
	_, err := q.Write(input)
	require.NoError(t, err)

	var readBytes atomic.Int64
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buffer := make([]byte, 17)
			for {
				n, err := q.Read(buffer)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						panic(err)
					}
					return
				}
				for _, value := range buffer[:n] {
					if value != 0xa5 {
						panic("read an unexpected byte")
					}
				}
				readBytes.Add(int64(n))
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(total), readBytes.Load())
	require.Zero(t, q.Len())
}

func TestChunkQueue_InvalidArguments(t *testing.T) {
	require.Panics(t, func() { NewChunkPool(0) })
	require.Panics(t, func() { NewChunkQueue(nil, 0) })
	require.Panics(t, func() { NewChunkQueue(NewChunkPool(1), -1) })
}

func TestChunkPool_BlockSize(t *testing.T) {
	pool := NewChunkPool(37)
	require.Equal(t, 37, pool.BlockSize())
	var nilPool *ChunkPool
	require.Zero(t, nilPool.BlockSize())
}

func TestChunk_RecyclePreservesStorage(t *testing.T) {
	data := []byte("abcdefgh")
	c := &chunk{
		data:  data,
		read:  2,
		write: 6,
		next:  &chunk{},
	}

	for range 2 {
		c.Recycle()
		require.Zero(t, c.read)
		require.Zero(t, c.write)
		require.Nil(t, c.next)
		require.Len(t, c.data, len(data))
		require.Equal(t, cap(data), cap(c.data))
		require.Same(t, &data[0], &c.data[0], "recycling must retain the block storage")
		require.Equal(t, data, c.data)
	}
}

func TestChunkPool_AllocatesAfterGarbageCollection(t *testing.T) {
	pool := NewChunkPool(8)
	q := NewChunkQueue(pool, 0)
	_, err := q.Write([]byte("12345678"))
	require.NoError(t, err)
	q.Reset()
	runtime.GC()

	_, err = q.Write([]byte("abcdefgh"))
	require.NoError(t, err)
	data := make([]byte, 8)
	_, err = q.Read(data)
	require.NoError(t, err)
	require.Equal(t, []byte("abcdefgh"), data)
}

func TestChunkQueue_ConcurrentReset(t *testing.T) {
	q := NewChunkQueue(NewChunkPool(64), 0)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				_, _ = q.Write([]byte("payload"))
				q.Reset()
			}
		}()
	}
	wg.Wait()
	q.Reset()
	require.Zero(t, q.Len())
}

func FuzzChunkQueue_RoundTrip(f *testing.F) {
	f.Add([]byte("hello"), uint8(1))
	f.Add([]byte("cross block data"), uint8(7))
	f.Fuzz(func(t *testing.T, data []byte, blockSize uint8) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		blockSize = blockSize%32 + 1
		q := NewChunkQueue(NewChunkPool(int(blockSize)), 0)
		for offset := 0; offset < len(data); {
			next := offset + int(blockSize)%11 + 1
			if next > len(data) {
				next = len(data)
			}
			written, err := q.Write(data[offset:next])
			require.NoError(t, err)
			require.Equal(t, next-offset, written)
			offset = next
		}

		got := make([]byte, len(data))
		read := 0
		for read < len(got) {
			next := read + int(blockSize)%13 + 1
			if next > len(got) {
				next = len(got)
			}
			n, err := q.Read(got[read:next])
			require.NoError(t, err)
			read += n
		}
		require.Equal(t, data, got)
		require.Zero(t, q.Len())
	})
}

func TestChunkQueue_LargeWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("large resident data test")
	}
	const (
		blockSize = 32 << 10
		size      = 256<<20 + 137 // Leave a partial final block.
	)
	q := NewChunkQueue(NewChunkPool(blockSize), 0)
	t.Cleanup(q.Reset)
	source := make([]byte, size)
	fillChunkQueueTestData(source)
	want := sha256.Sum256(source)

	n, err := q.Write(source)
	require.NoError(t, err)
	require.Equal(t, size, n)
	require.Equal(t, size, q.Len())
	clear(source) // The entire large write must be independent of its source.

	got := sha256.New()
	buffer := make([]byte, 48<<10+11)
	for remaining := size; remaining > 0; {
		n, err = q.Read(buffer)
		require.NoError(t, err)
		require.Equal(t, min(remaining, len(buffer)), n)
		_, _ = got.Write(buffer[:n])
		remaining -= n
		require.Equal(t, remaining, q.Len())
	}
	require.Equal(t, want[:], got.Sum(nil), "large write changed or reordered bytes")
	n, err = q.Read(buffer)
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
}

func TestChunkQueue_LargeBacklog(t *testing.T) {
	if testing.Short() {
		t.Skip("long block chain test")
	}
	const (
		blockSize = 257
		blocks    = 100_000
		size      = blockSize*blocks + 19
	)
	pool := NewChunkPool(blockSize)
	q := NewChunkQueue(pool, 0)
	t.Cleanup(q.Reset)
	input := make([]byte, 8191)
	fillChunkQueueTestData(input)

	// Repeat growth, partial consumption, Reset and refill with a shared pool.
	for cycle := range 2 {
		target := size
		if cycle == 0 {
			target = size/2 + 3
		}
		want := sha256.New()
		for offset := 0; offset < size; {
			binary.LittleEndian.PutUint64(input, uint64(offset))
			input[8] = byte(cycle)
			data := input[:min(len(input), size-offset)]
			n, err := q.Write(data)
			require.NoError(t, err)
			require.Equal(t, len(data), n)
			if offset < target {
				_, _ = want.Write(data[:min(n, target-offset)])
			}
			offset += n
		}
		require.Equal(t, size, q.Len())

		got := sha256.New()
		output := make([]byte, 64<<10+7)
		for offset := 0; offset < target; {
			data := output[:min(len(output), target-offset)]
			n, err := q.Read(data)
			require.NoError(t, err)
			require.Equal(t, len(data), n)
			_, _ = got.Write(data[:n])
			offset += n
			require.Equal(t, size-offset, q.Len())
		}
		require.Equal(t, want.Sum(nil), got.Sum(nil), "chain contains stale or reordered data")
		q.Reset()
		q.Reset()
		require.Zero(t, q.Len())
		n, err := q.Read(output)
		require.Zero(t, n)
		require.ErrorIs(t, err, io.EOF)
	}
}

func TestChunkQueue_LargeBoundedWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("large bounded data test")
	}
	const limit = 64<<20 + 23
	q := NewChunkQueue(NewChunkPool(32<<10), limit)
	t.Cleanup(q.Reset)
	input := make([]byte, limit+101)
	fillChunkQueueTestData(input)

	n, err := q.Write(input)
	require.ErrorIs(t, err, ErrChunkQueueFull)
	require.Equal(t, limit, n)
	require.Equal(t, limit, q.Len())
	n, err = q.Write(input[n:])
	require.ErrorIs(t, err, ErrChunkQueueFull)
	require.Zero(t, n)

	output := make([]byte, 1<<20+13)
	n, err = q.Read(output)
	require.NoError(t, err)
	require.Equal(t, len(output), n)
	require.Equal(t, sha256.Sum256(input[:n]), sha256.Sum256(output))
	want := sha256.Sum256(input[n:])
	_, err = q.Write(input[limit:])
	require.NoError(t, err)
	require.Equal(t, len(input)-n, q.Len())

	got := sha256.New()
	for remaining := q.Len(); remaining > 0; {
		n, err = q.Read(output)
		require.NoError(t, err)
		require.Equal(t, min(len(output), remaining), n)
		_, _ = got.Write(output[:n])
		remaining -= n
	}
	require.Equal(t, want[:], got.Sum(nil), "partial write and refill lost or repeated bytes")
	require.Zero(t, q.Len())
}

func TestChunkQueue_LargeConcurrentStream(t *testing.T) {
	if testing.Short() {
		t.Skip("large cumulative stream test")
	}
	const (
		size  = 1<<30 + 113
		limit = 1<<20 + 7
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	q := NewChunkQueue(NewChunkPool(32<<10), limit)
	t.Cleanup(q.Reset)
	readable := make(chan struct{}, 1)
	writable := make(chan struct{}, 1)
	written := make(chan streamResult, 1)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		written <- produceChunkQueueStream(ctx, q, size, readable, writable)
	}()
	// A failing assertion must cancel and join the producer too.
	defer func() {
		cancel()
		select {
		case <-writerDone:
		case <-time.After(5 * time.Second):
			t.Error("stream producer did not exit after cancellation")
		}
	}()

	got := sha256.New()
	output := make([]byte, 48<<10+11)
	for offset := 0; offset < size; {
		require.NoError(t, ctx.Err(), "stream stalled after %d bytes", offset)
		n, err := q.Read(output)
		if errors.Is(err, io.EOF) {
			require.Zero(t, n)
			select {
			case <-readable:
			case <-writerDone:
				if q.Len() == 0 {
					want := <-written
					t.Fatalf("stream producer exited with %d of %d bytes consumed: %v", offset, size, want.err)
				}
			case <-ctx.Done():
				t.Fatalf("stream read stalled after %d bytes: %v", offset, ctx.Err())
			}
			continue
		}
		require.NoError(t, err)
		require.Positive(t, n)
		_, _ = got.Write(output[:n])
		offset += n
		require.LessOrEqual(t, offset, size)
		require.LessOrEqual(t, q.Len(), limit)
		select {
		case writable <- struct{}{}:
		default:
		}
	}

	select {
	case want := <-written:
		require.NoError(t, want.err)
		require.Equal(t, want.digest, got.Sum(nil), "stream lost, repeated or reordered bytes")
	case <-ctx.Done():
		t.Fatal("stream producer did not complete:", ctx.Err())
	}
	require.Zero(t, q.Len())
}

type streamResult struct {
	digest []byte
	err    error
}

// produceChunkQueueStream feeds one deterministic byte stream into q until size
// bytes are accepted, waiting for the consumer whenever the queue is full.
func produceChunkQueueStream(ctx context.Context, q *ChunkQueue, size int, readable, writable chan struct{}) streamResult {
	input := make([]byte, 64<<10+13)
	fillChunkQueueTestData(input)
	digest := sha256.New()
	for offset := 0; offset < size; {
		binary.LittleEndian.PutUint64(input, uint64(offset))
		data := input[:min(len(input), size-offset)]
		for len(data) > 0 {
			if err := ctx.Err(); err != nil {
				return streamResult{err: err}
			}
			n, err := q.Write(data)
			if n < 0 || n > len(data) || (err != nil && !errors.Is(err, ErrChunkQueueFull)) || (err == nil && n != len(data)) {
				return streamResult{err: fmt.Errorf("invalid Write result: n=%d, input=%d: %w", n, len(data), err)}
			}
			if n > 0 {
				_, _ = digest.Write(data[:n])
				data = data[n:]
				offset += n
				select {
				case readable <- struct{}{}:
				default:
				}
			}
			if errors.Is(err, ErrChunkQueueFull) {
				select {
				case <-writable:
				case <-ctx.Done():
					return streamResult{err: ctx.Err()}
				}
			}
		}
	}
	return streamResult{digest: digest.Sum(nil)}
}

func fillChunkQueueTestData(data []byte) {
	// A deterministic non-constant sequence makes ordering and stale data
	// visible without keeping a second resident copy of the entire stream.
	state := uint64(1)
	for i := range data {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		data[i] = byte(state)
	}
}

func BenchmarkChunkQueue_WriteRead(b *testing.B) {
	for _, scenario := range []struct {
		name string
		size int
	}{
		{"1KiB", 1 << 10},
		{"64KiB", 64 << 10},
		{"1MiB", 1 << 20},
		{"64MiB", 64 << 20},
		{"256MiB", 256 << 20},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			q := NewChunkQueue(NewChunkPool(32<<10), 0)
			b.Cleanup(q.Reset)
			input := make([]byte, scenario.size)
			fillChunkQueueTestData(input)
			output := make([]byte, 64<<10)
			if _, err := q.Write(input); err != nil {
				b.Fatal(err)
			}
			readChunkQueueBenchmark(b, q, output, scenario.size)
			b.SetBytes(int64(scenario.size))
			b.ReportAllocs()

			for b.Loop() {
				if n, err := q.Write(input); err != nil || n != len(input) {
					b.Fatalf("Write: n=%d, error=%v", n, err)
				}
				readChunkQueueBenchmark(b, q, output, scenario.size)
			}
		})
	}
}

// FreshPool includes pool/queue creation and first allocation of every block.
func BenchmarkChunkQueue_FreshPool(b *testing.B) {
	for _, scenario := range []struct {
		name string
		size int
	}{
		{"1MiB", 1 << 20},
		{"64MiB", 64 << 20},
		{"256MiB", 256 << 20},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			input := make([]byte, scenario.size)
			fillChunkQueueTestData(input)
			output := make([]byte, 64<<10)
			b.SetBytes(int64(scenario.size))
			b.ReportAllocs()
			for b.Loop() {
				q := NewChunkQueue(NewChunkPool(32<<10), 0)
				if n, err := q.Write(input); err != nil || n != len(input) {
					b.Fatalf("Write: n=%d, error=%v", n, err)
				}
				readChunkQueueBenchmark(b, q, output, scenario.size)
			}
		})
	}
}

// Backlog keeps the resident data size constant while reading/writing 1KiB.
func BenchmarkChunkQueue_Backlog(b *testing.B) {
	for _, scenario := range []struct {
		name string
		size int
	}{
		{"Empty", 0},
		{"1MiB", 1 << 20},
		{"64MiB", 64 << 20},
		{"256MiB", 256 << 20},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			q := NewChunkQueue(NewChunkPool(32<<10), 0)
			b.Cleanup(q.Reset)
			seed := make([]byte, 64<<10)
			fillChunkQueueTestData(seed)
			for remaining := scenario.size; remaining > 0; {
				n, err := q.Write(seed[:min(len(seed), remaining)])
				if err != nil || n == 0 {
					b.Fatalf("seed: n=%d, error=%v", n, err)
				}
				remaining -= n
			}
			input := make([]byte, 1<<10)
			fillChunkQueueTestData(input)
			output := make([]byte, len(input))
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for b.Loop() {
				if n, err := q.Write(input); err != nil || n != len(input) {
					b.Fatalf("Write: n=%d, error=%v", n, err)
				}
				if n, err := q.Read(output); err != nil || n != len(output) {
					b.Fatalf("Read: n=%d, error=%v", n, err)
				}
			}
			if q.Len() != scenario.size {
				b.Fatalf("backlog: got %d bytes, want %d", q.Len(), scenario.size)
			}
		})
	}
}

// WriteReset includes copying the input and recycling the entire block chain.
func BenchmarkChunkQueue_WriteReset(b *testing.B) {
	for _, scenario := range []struct {
		name string
		size int
	}{
		{"1MiB", 1 << 20},
		{"64MiB", 64 << 20},
		{"256MiB", 256 << 20},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			q := NewChunkQueue(NewChunkPool(32<<10), 0)
			b.Cleanup(q.Reset)
			input := make([]byte, scenario.size)
			fillChunkQueueTestData(input)
			if _, err := q.Write(input); err != nil {
				b.Fatal(err)
			}
			q.Reset()
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for b.Loop() {
				if n, err := q.Write(input); err != nil || n != len(input) {
					b.Fatalf("Write: n=%d, error=%v", n, err)
				}
				q.Reset()
			}
		})
	}
}

func BenchmarkChunkQueue_SharedPool(b *testing.B) {
	pool := NewChunkPool(32 << 10)
	input := make([]byte, 1<<10)
	fillChunkQueueTestData(input)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		q := NewChunkQueue(pool, 0)
		output := make([]byte, len(input))
		for pb.Next() {
			if n, err := q.Write(input); err != nil || n != len(input) {
				b.Errorf("Write: n=%d, error=%v", n, err)
				return
			}
			if n, err := q.Read(output); err != nil || n != len(output) {
				b.Errorf("Read: n=%d, error=%v", n, err)
				return
			}
		}
		q.Reset()
	})
}

func readChunkQueueBenchmark(b *testing.B, q *ChunkQueue, output []byte, size int) {
	for remaining := size; remaining > 0; {
		want := min(len(output), remaining)
		n, err := q.Read(output[:want])
		if err != nil || n != want {
			b.Fatalf("Read: n=%d, want=%d, error=%v", n, want, err)
		}
		remaining -= n
	}
}
