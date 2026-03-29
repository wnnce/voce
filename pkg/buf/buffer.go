package buf

type Buffer struct {
	Buf        []byte
	RecycleCap int
}

func (b *Buffer) Set(payload []byte) {
	required := len(payload)
	if required == 0 {
		return
	}
	if cap(b.Buf) < required {
		b.Buf = make([]byte, required)
	} else {
		b.Buf = b.Buf[:required]
	}
	copy(b.Buf, payload)
}

func (b *Buffer) Recycle() {
	if b.RecycleCap > 0 && cap(b.Buf) > b.RecycleCap {
		b.Buf = nil
	} else {
		b.Buf = b.Buf[:0]
	}
}

func Clone[S ~[]E, E any](src S) S {
	ans := make([]E, len(src))
	copy(ans, src)
	return ans
}
