package buf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuffer_Set(t *testing.T) {
	t.Run("empty payload", func(t *testing.T) {
		b := &Buffer{}
		b.Set([]byte(""))
		assert.Nil(t, b.Buf)
	})

	t.Run("allocate new buf", func(t *testing.T) {
		b := &Buffer{}
		payload := []byte("hello")
		b.Set(payload)
		assert.Equal(t, payload, b.Buf)
		assert.Len(t, b.Buf, 5)
	})

	t.Run("reslice existing buf", func(t *testing.T) {
		b := &Buffer{Buf: make([]byte, 10)}
		oldCap := cap(b.Buf)
		payload := []byte("hi")
		b.Set(payload)
		assert.Equal(t, payload, b.Buf)
		assert.Equal(t, oldCap, cap(b.Buf))
	})
}

func TestBuffer_Recycle(t *testing.T) {
	t.Run("no recycle cap", func(t *testing.T) {
		b := &Buffer{Buf: make([]byte, 10)}
		b.Buf = b.Buf[:5]
		b.Recycle()
		assert.NotNil(t, b.Buf)
		assert.Empty(t, b.Buf)
		assert.Equal(t, 10, cap(b.Buf))
	})

	t.Run("within recycle cap", func(t *testing.T) {
		b := &Buffer{Buf: make([]byte, 10), RecycleCap: 15}
		b.Recycle()
		assert.NotNil(t, b.Buf)
		assert.Equal(t, 10, cap(b.Buf))
	})

	t.Run("exceed recycle cap", func(t *testing.T) {
		b := &Buffer{Buf: make([]byte, 20), RecycleCap: 15}
		b.Recycle()
		assert.Nil(t, b.Buf)
	})
}
