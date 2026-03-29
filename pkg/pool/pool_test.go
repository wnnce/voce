package pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockRecycler struct {
	recycled bool
}

func (m *mockRecycler) Recycle() {
	m.recycled = true
}

func TestTypedPool(t *testing.T) {
	factory := func() *mockRecycler {
		return &mockRecycler{}
	}
	p := NewTypedPool(factory)

	// Test Acquire
	obj := p.Acquire()
	assert.NotNil(t, obj)
	assert.False(t, obj.recycled)

	// Test Release calls Recycle
	p.Release(obj)
	assert.True(t, obj.recycled)

	// Test reuse after Release
	obj2 := p.Acquire()
	assert.NotNil(t, obj2)
}
