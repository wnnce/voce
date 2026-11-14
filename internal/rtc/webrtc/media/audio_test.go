package media

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateSinePCM(samples int, freq float64) []byte {
	buf := make([]byte, samples*BytesPerSample)
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(AudioSampleRate)
		val := int16(math.Sin(2*math.Pi*freq*t) * 16384)
		buf[2*i] = byte(val)
		buf[2*i+1] = byte(val >> 8)
	}
	return buf
}

func TestAudioCodec_RoundTrip(t *testing.T) {
	encoder, err := NewAudioEncoder()
	require.NoError(t, err)
	require.NotNil(t, encoder)

	decoder, err := NewAudioDecoder()
	require.NoError(t, err)
	require.NotNil(t, decoder)

	pcm := generateSinePCM(SamplesPerFrame, 440)
	encoded, err := encoder.Encode(pcm)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, err := decoder.Decode(encoded)
	require.NoError(t, err)
	assert.Len(t, decoded, PCMFrameSize)
}

func TestAudioCodec_PadPartialFrame(t *testing.T) {
	encoder, err := NewAudioEncoder()
	require.NoError(t, err)

	decoder, err := NewAudioDecoder()
	require.NoError(t, err)

	shortPCM := generateSinePCM(SamplesPerFrame/2, 1000)
	encoded, err := encoder.Encode(shortPCM)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, err := decoder.Decode(encoded)
	require.NoError(t, err)
	assert.Len(t, decoded, PCMFrameSize)
}

func BenchmarkAudioEncoder_Encode(b *testing.B) {
	encoder, err := NewAudioEncoder()
	require.NoError(b, err)

	pcm := generateSinePCM(SamplesPerFrame, 440)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoder.Encode(pcm)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAudioDecoder_Decode(b *testing.B) {
	encoder, err := NewAudioEncoder()
	require.NoError(b, err)

	decoder, err := NewAudioDecoder()
	require.NoError(b, err)

	pcm := generateSinePCM(SamplesPerFrame, 440)
	encoded, err := encoder.Encode(pcm)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := decoder.Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}
