package pool

import (
	"sync"
)

type Recycler interface {
	Recycle()
}

type TypedPool[T Recycler] struct {
	pool *sync.Pool
}

func NewTypedPool[T Recycler](factory func() T) *TypedPool[T] {
	return &TypedPool[T]{
		pool: &sync.Pool{
			New: func() any {
				return factory()
			},
		},
	}
}

func (p *TypedPool[T]) Acquire() T {
	return p.pool.Get().(T)
}

func (p *TypedPool[T]) Release(v T) {
	v.Recycle()
	p.pool.Put(v)
}
