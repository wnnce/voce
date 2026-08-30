package schema

import (
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestVideo_Lifecycle(t *testing.T) {
	t.Run("Reference Counting", func(t *testing.T) {
		v := NewVideo("ref", 480, 640, 33*time.Millisecond).(*builtinVideo)
		v.Retain()
		v.Release()
		assert.Equal(t, "ref", v.Name())
		v.Release()
		assert.Empty(t, v.Name())
	})

	t.Run("Copy-on-write Across Resolutions", func(t *testing.T) {
		v1 := NewVideo("sd", 480, 640, 0)
		v2 := NewVideo("hd", 720, 1280, 0)

		v1.Release()

		// Test Mutable clone across resolution
		ro := v2.ReadOnly()
		mutable := ro.Mutable()
		assert.NotSame(t, ro, mutable)
		assert.Equal(t, v2.Name(), mutable.Name())
		assert.Equal(t, v2.Width(), mutable.Width())
		assert.Equal(t, v2.Height(), mutable.Height())

		mutable.Release()
		ro.Release()
	})
}

func TestVideo_ContiguousSliceMechanism(t *testing.T) {
	t.Run("YUV Offsets and Data Contiguity", func(t *testing.T) {
		height, width := 4, 4 // tiny frame for easy math
		pixels := height * width
		uSize := pixels / 4
		vSize := pixels / 4
		expectedTotal := pixels + uSize + vSize

		v := NewVideo("test", height, width, 33*time.Millisecond)
		defer v.Release()

		// Fill with test patterns
		yData := make([]byte, pixels)
		uData := make([]byte, uSize)
		vData := make([]byte, vSize)
		for i := 0; i < pixels; i++ {
			yData[i] = 'Y'
		}
		for i := 0; i < uSize; i++ {
			uData[i] = 'U'
		}
		for i := 0; i < vSize; i++ {
			vData[i] = 'V'
		}

		v.SetYUV(yData, uData, vData)

		// Check lengths
		assert.Len(t, v.YBytes(), pixels)
		assert.Len(t, v.UBytes(), uSize)
		assert.Len(t, v.VBytes(), vSize)

		// Check values
		assert.Equal(t, yData, v.YBytes())
		assert.Equal(t, uData, v.UBytes())
		assert.Equal(t, vData, v.VBytes())

		// CRITICAL: Check that they are slices of the same underlying array
		yPtr := getSlicePtr(v.YBytes())
		uPtr := getSlicePtr(v.UBytes())
		vPtr := getSlicePtr(v.VBytes())

		// Verify offsets
		assert.Equal(t, yPtr+uintptr(pixels), uPtr, "U should follow Y immediately")
		assert.Equal(t, uPtr+uintptr(uSize), vPtr, "V should follow U immediately")

		// Verify total underlying data length
		inner := v.(*builtinVideo)
		assert.Len(t, inner.data, expectedTotal)
	})
}

func TestVideo_ReadOnlyAndMutable(t *testing.T) {
	t.Run("ReadOnly Transition", func(t *testing.T) {
		v := NewVideo("test", 10, 10, 33*time.Millisecond)
		defer v.Release()

		ro := v.ReadOnly()
		assert.Equal(t, v.Name(), ro.Name())

		// If we use cast, we should see readonly is true
		inner := ro.(*builtinVideo)
		assert.True(t, inner.readonly.Load())
	})

	t.Run("Mutable Copy-on-Write", func(t *testing.T) {
		v := NewVideo("master", 10, 10, 33*time.Millisecond)
		defer v.Release()
		_ = v.Set("meta", "original")

		ro := v.ReadOnly()

		// Mutable on ReadOnly should return a NEW copy
		writable := ro.Mutable()
		defer writable.Release()

		assert.NotSame(t, ro, writable)
		assert.Equal(t, "master", writable.Name())

		val, _ := writable.Get("meta")
		assert.Equal(t, "original", val)

		// Modify new
		_ = writable.Set("meta", "changed")
		valRO, _ := ro.Get("meta")
		assert.Equal(t, "original", valRO, "Original should be unchanged")
	})

	t.Run("Mutable returns self if not ReadOnly", func(t *testing.T) {
		v := NewVideo("test", 10, 10, 33*time.Millisecond)
		defer v.Release()

		writable := v.(*builtinVideo).Mutable()
		assert.Same(t, v, writable)
	})
}

// Helper to get the underlying pointer of a slice for contiguity checks
func getSlicePtr(b []byte) uintptr {
	if len(b) == 0 {
		return 0
	}
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	return hdr.Data
}
