package syncx

import (
	"fmt"
	"hash/fnv"
	"sync"
)

type HashFunc[K comparable] func(key K) uint64

type shardedBucket[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]V
}

type ShardedMap[K comparable, V any] struct {
	shards []shardedBucket[K, V]
	hash   HashFunc[K]
}

func NewShardedMap[K comparable, V any](shardCount int, hash HashFunc[K]) *ShardedMap[K, V] {
	if shardCount <= 0 {
		shardCount = 1
	}
	shards := make([]shardedBucket[K, V], shardCount)
	for i := range shards {
		shards[i].entries = make(map[K]V)
	}
	if hash == nil {
		hash = defaultHash[K]
	}
	return &ShardedMap[K, V]{
		shards: shards,
		hash:   hash,
	}
}

func (m *ShardedMap[K, V]) Store(key K, value V) {
	shard := m.shard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.entries[key] = value
}

func (m *ShardedMap[K, V]) Load(key K) (V, bool) {
	shard := m.shard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	value, ok := shard.entries[key]
	return value, ok
}

func (m *ShardedMap[K, V]) Delete(key K) {
	shard := m.shard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.entries, key)
}

func (m *ShardedMap[K, V]) Update(key K, fn func(value V, ok bool) (V, bool)) bool {
	shard := m.shard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	value, ok := shard.entries[key]
	next, keep := fn(value, ok)
	if !keep {
		if ok {
			delete(shard.entries, key)
		}
		return ok
	}
	shard.entries[key] = next
	return true
}

func (m *ShardedMap[K, V]) Range(fn func(key K, value V) bool) {
	for i := range m.shards {
		shard := &m.shards[i]
		shard.mu.RLock()
		snapshot := make([]struct {
			key   K
			value V
		}, 0, len(shard.entries))
		for key, value := range shard.entries {
			snapshot = append(snapshot, struct {
				key   K
				value V
			}{key: key, value: value})
		}
		shard.mu.RUnlock()
		for _, item := range snapshot {
			if !fn(item.key, item.value) {
				return
			}
		}
	}
}

func (m *ShardedMap[K, V]) shard(key K) *shardedBucket[K, V] {
	if len(m.shards) == 1 {
		return &m.shards[0]
	}
	return &m.shards[m.hash(key)%uint64(len(m.shards))]
}

func defaultHash[K comparable](key K) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprint(key)))
	return h.Sum64()
}
