package buf

import (
	"errors"
	"io"
	"sync"

	"github.com/wnnce/voce/pkg/pool"
)

var (
	// ErrChunkQueueFull means a bounded queue accepted only part of the input.
	ErrChunkQueueFull = errors.New("buf: chunk queue full")
)

// ChunkPool reuses fixed-size byte blocks shared by one or more ChunkQueues.
// sync.Pool may discard cached blocks during a garbage collection, so callers
// must treat it as an allocation optimization rather than a capacity limit.
type ChunkPool struct {
	blockSize int
	blocks    *pool.TypedPool[*chunk]
}

// NewChunkPool creates a pool whose blocks have blockSize bytes. A block size
// is fixed for the lifetime of the pool so queues sharing it can reuse blocks.
func NewChunkPool(blockSize int) *ChunkPool {
	if blockSize <= 0 {
		panic("buf: chunk block size must be positive")
	}

	return &ChunkPool{
		blockSize: blockSize,
		blocks: pool.NewTypedPool(func() *chunk {
			return &chunk{data: make([]byte, blockSize)}
		}),
	}
}

// BlockSize returns the capacity of every block acquired from p.
func (p *ChunkPool) BlockSize() int {
	if p == nil {
		return 0
	}
	return p.blockSize
}

// ChunkQueue is a concurrent byte stream backed by a linked list of fixed-size
// blocks. Write copies input bytes into the queue; Read copies bytes out, so no
// caller can retain a slice that becomes invalid when a block is recycled.
//
// A limit of zero means unlimited growth. For a bounded queue, Write
// accepts as much as fits and returns ErrChunkQueueFull when input remains.
// The queue never grows a block or copies already queued bytes during growth.
type ChunkQueue struct {
	mu sync.Mutex

	pool *ChunkPool

	head *chunk
	tail *chunk

	size  int // Number of unread bytes.
	limit int // Maximum unread bytes; zero means unlimited.
}

// NewChunkQueue creates a queue backed by pool. limit is the maximum number of
// unread bytes; zero allows the queue to grow as blocks are added.
func NewChunkQueue(pool *ChunkPool, limit int) *ChunkQueue {
	if pool == nil {
		panic("buf: chunk pool is nil")
	}
	if limit < 0 {
		panic("buf: chunk queue limit must not be negative")
	}
	return &ChunkQueue{pool: pool, limit: limit}
}

// Write copies p into the queue. If the queue is bounded and cannot accept all
// bytes, it returns the number copied and ErrChunkQueueFull.
func (q *ChunkQueue) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	q.mu.Lock()
	count := len(p)
	if q.limit > 0 {
		available := q.limit - q.size
		if available <= 0 {
			q.mu.Unlock()
			return 0, ErrChunkQueueFull
		}
		if count > available {
			count = available
		}
	}

	written := 0
	for written < count {
		if q.tail == nil || q.tail.write == len(q.tail.data) {
			block := q.pool.blocks.Acquire()
			if q.tail == nil {
				q.head, q.tail = block, block
			} else {
				q.tail.next = block
				q.tail = block
			}
		}

		block := q.tail
		n := copy(block.data[block.write:], p[written:count])
		block.write += n
		written += n
		q.size += n
	}
	q.mu.Unlock()

	if written != len(p) {
		return written, ErrChunkQueueFull
	}
	return written, nil
}

// Read copies up to len(dst) bytes from the queue. It returns io.EOF when the
// queue is currently empty; EOF means no data is available now, not that the
// queue cannot receive more data later.
func (q *ChunkQueue) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}

	q.mu.Lock()
	if q.head == nil {
		q.mu.Unlock()
		return 0, io.EOF
	}

	read := 0
	var recycledHead, recycledTail *chunk
	for read < len(dst) && q.head != nil {
		block := q.head
		n := copy(dst[read:], block.data[block.read:block.write])
		block.read += n
		read += n
		q.size -= n

		if block.read != block.write {
			break
		}

		q.head = block.next
		block.next = nil
		if q.head == nil {
			q.tail = nil
		}
		if recycledHead == nil {
			recycledHead = block
		} else {
			recycledTail.next = block
		}
		recycledTail = block
	}
	q.mu.Unlock()

	q.releaseList(recycledHead)
	return read, nil
}

// Len returns the number of bytes currently available to Read.
func (q *ChunkQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

// Reset discards all queued bytes and returns every block to the shared pool.
// It is safe to call concurrently with Write and Read.
func (q *ChunkQueue) Reset() {
	q.mu.Lock()
	head := q.head
	q.head = nil
	q.tail = nil
	q.size = 0
	q.mu.Unlock()

	q.releaseList(head)
}

func (q *ChunkQueue) releaseList(head *chunk) {
	for head != nil {
		next := head.next
		q.pool.blocks.Release(head)
		head = next
	}
}

type chunk struct {
	data        []byte
	read, write int
	next        *chunk
}

func (c *chunk) Recycle() {
	c.read = 0
	c.write = 0
	c.next = nil
}
