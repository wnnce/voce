package media

import (
	"fmt"
	"time"

	"github.com/hraban/opus"
)

const (
	AudioSampleRate       = 16000
	AudioChannels         = 1
	PacingInterval        = 20 * time.Millisecond
	SamplesPerFrame       = AudioSampleRate * 20 / 1000      // 320 samples per 20ms frame
	BytesPerSample        = 2                                // 16-bit PCM (2 bytes per sample)
	PCMFrameSize          = SamplesPerFrame * BytesPerSample // 640 bytes per 20ms frame
	MaxDecodedFrameSample = 5760                             // 120ms max Opus frame size
	OpusSDPFmtpLine       = "minptime=10;useinbandfec=1"
)

// AudioEncoder encodes 16kHz mono PCM16 audio frames into Opus packets for WebRTC outbound streaming.
// It maintains preallocated buffers to achieve zero allocation on hot paths.
type AudioEncoder struct {
	encoder *opus.Encoder
	pcmBuf  []int16
	outBuf  []byte
	padBuf  []byte
}

// NewAudioEncoder initializes an Opus encoder configured for real-time VoIP.
func NewAudioEncoder() (*AudioEncoder, error) {
	encoder, err := opus.NewEncoder(AudioSampleRate, AudioChannels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("media: new opus encoder: %w", err)
	}

	return &AudioEncoder{
		encoder: encoder,
		pcmBuf:  make([]int16, SamplesPerFrame),
		outBuf:  make([]byte, 1024),
		padBuf:  make([]byte, PCMFrameSize),
	}, nil
}

// Encode converts raw 16kHz mono PCM bytes into an Opus packet.
// rawPCM should be PCMFrameSize (640 bytes). If shorter, it is padded with silence.
// The returned slice references the internal buffer and is valid until the next call.
func (e *AudioEncoder) Encode(rawPCM []byte) ([]byte, error) {
	var src []byte
	if len(rawPCM) == PCMFrameSize {
		src = rawPCM
	} else {
		clear(e.padBuf)
		copy(e.padBuf, rawPCM)
		src = e.padBuf
	}

	for i := 0; i < SamplesPerFrame; i++ {
		e.pcmBuf[i] = int16(src[2*i]) | (int16(src[2*i+1]) << 8)
	}

	n, err := e.encoder.Encode(e.pcmBuf, e.outBuf)
	if err != nil {
		return nil, fmt.Errorf("media: opus encode: %w", err)
	}
	return e.outBuf[:n], nil
}

// AudioDecoder decodes Opus packets received from a remote WebRTC track into 16kHz mono PCM16 bytes.
// Each incoming track must own an independent decoder instance to maintain jitter and PLC state.
type AudioDecoder struct {
	decoder *opus.Decoder
	pcmBuf  []int16
	rawBuf  []byte
}

// NewAudioDecoder initializes an independent Opus decoder for a remote track.
func NewAudioDecoder() (*AudioDecoder, error) {
	decoder, err := opus.NewDecoder(AudioSampleRate, AudioChannels)
	if err != nil {
		return nil, fmt.Errorf("media: new opus decoder: %w", err)
	}
	return &AudioDecoder{
		decoder: decoder,
		pcmBuf:  make([]int16, MaxDecodedFrameSample),
		rawBuf:  make([]byte, MaxDecodedFrameSample*BytesPerSample),
	}, nil
}

// Decode decodes an Opus packet into 16kHz mono PCM16 bytes.
// The returned slice references the internal buffer and is valid until the next call.
func (d *AudioDecoder) Decode(opusData []byte) ([]byte, error) {
	n, err := d.decoder.Decode(opusData, d.pcmBuf)
	if err != nil {
		return nil, fmt.Errorf("media: opus decode: %w", err)
	}
	byteLen := n * BytesPerSample
	for i := 0; i < n; i++ {
		sample := d.pcmBuf[i]
		d.rawBuf[2*i] = byte(sample)
		d.rawBuf[2*i+1] = byte(sample >> 8)
	}
	return d.rawBuf[:byteLen], nil
}
