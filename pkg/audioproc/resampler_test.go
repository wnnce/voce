package audioproc

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateSineWave generates a mono sine wave of given duration and frequency at a specific sample rate.
func generateSineWave(rate, freq int, durationMs int) []byte {
	samples := rate * durationMs / 1000
	buf := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		sample := int16(math.MaxInt16 * math.Sin(2*math.Pi*float64(freq)*float64(i)/float64(rate)))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
	}
	return buf
}

func TestNewResampler(t *testing.T) {
	r, err := NewResampler(44100, 2, 16000, 1)
	require.NoError(t, err)
	assert.NotNil(t, r)
	r.Close()

	// Error case (unsupported rate or channels)
	r, err = NewResampler(0, 1, 16000, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInitFailed)
}

func TestResample_SampleRates(t *testing.T) {
	tests := []struct {
		name       string
		srcRate    int
		dstRate    int
		durationMs int
	}{
		{"44.1k to 16k", 44100, 16000, 100},
		{"24k to 16k", 24000, 16000, 100},
		{"16k to 44.1k", 16000, 44100, 100},
		{"48k to 16k", 48000, 16000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewResampler(tt.srcRate, 1, tt.dstRate, 1)
			require.NoError(t, err)
			defer r.Close()

			srcPCM := generateSineWave(tt.srcRate, 440, tt.durationMs)
			dstPCM, err := r.Resample(srcPCM)

			require.NoError(t, err)
			assert.NotNil(t, dstPCM)

			expectedSamples := (len(srcPCM) / 2) * tt.dstRate / tt.srcRate
			assert.InDelta(t, expectedSamples, len(dstPCM)/2, 64, "Output sample count mismatch for "+tt.name)
		})
	}
}

func TestResample_StereoToMono(t *testing.T) {
	rate := 16000
	r, _ := NewResampler(rate, 2, rate, 1)
	defer r.Close()

	// Create stereo PCM where left is tone, right is silence
	samples := rate * 100 / 1000
	srcPCM := make([]byte, samples*2*2) // 4 bytes per stereo sample (2xS16)
	tone := generateSineWave(rate, 440, 100)
	for i := 0; i < samples; i++ {
		// Left channel
		srcPCM[i*4] = tone[i*2]
		srcPCM[i*4+1] = tone[i*2+1]
		// Right channel (silence)
		srcPCM[i*4+2] = 0
		srcPCM[i*4+3] = 0
	}

	dstPCM, err := r.Resample(srcPCM)
	require.NoError(t, err)
	assert.Len(t, dstPCM, samples*2, "Output should be half size (stereo to mono)")

	sampleValue := int16(binary.LittleEndian.Uint16(dstPCM[200:202]))
	assert.NotZero(t, sampleValue)
}

func TestResample_MemoryReuse(t *testing.T) {
	r, _ := NewResampler(16000, 1, 16000, 1)
	defer r.Close()

	pcm1 := generateSineWave(16000, 440, 20) // 20ms
	res1, _ := r.Resample(pcm1)
	addr1 := &res1[0]

	pcm2 := generateSineWave(16000, 880, 20) // 20ms
	res2, _ := r.Resample(pcm2)
	addr2 := &res2[0]

	assert.Same(t, addr1, addr2, "Memory should be reused")

	largePCM := generateSineWave(16000, 440, 100) // 100ms
	res3, _ := r.Resample(largePCM)
	addr3 := &res3[0]

	res4, _ := r.Resample(largePCM)
	addr4 := &res4[0]
	assert.Same(t, addr3, addr4, "Memory should be reused after growth")
}

func TestResample_EmptyInput(t *testing.T) {
	r, _ := NewResampler(16000, 1, 8000, 1)
	defer r.Close()

	res, err := r.Resample([]byte{})
	require.NoError(t, err)
	assert.Nil(t, res)
}

func BenchmarkResample(b *testing.B) {
	srcRate := 44100
	dstRate := 16000
	r, _ := NewResampler(srcRate, 1, dstRate, 1)
	defer r.Close()

	input := generateSineWave(srcRate, 440, 20)
	for i := 0; i < 5; i++ {
		_, _ = r.Resample(input)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := r.Resample(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
