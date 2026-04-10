package ten_vad

import (
	"context"
	"encoding/binary"
	"log/slog"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

const (
	defaultFrameBytes      = 512
	defaultFrameDurationMs = 16
	defaultMinSpeechMs     = 150
	defaultMinSilenceMs    = 600
	defaultThreshold       = 0.5
)

type Processor interface {
	Process(speechFrame []int16) (float32, bool, error)
	FrameSize() int
	Close() error
}

//nolint:lll // struct tags are intentionally long for jsonschema
type Config struct {
	Threshold            float32 `json:"threshold" jsonschema:"description=Speech probability threshold,default=0.5,minimum=0,maximum=1"`
	MinSpeechDurationMs  int     `json:"min_speech_duration_ms" jsonschema:"description=Minimum continuous speech duration (ms),default=150,minimum=20"`
	MinSilenceDurationMs int     `json:"min_silence_duration_ms" jsonschema:"description=Minimum continuous silence duration (ms),default=600,minimum=20"`
	FrameBytes           int     `json:"frame_bytes" jsonschema:"description=Internal frame size in bytes,default=512,enum=512"`
}

func (c *Config) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(c)
}

func (c *Config) Decode(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return sonic.Unmarshal(data, c)
}

type Plugin struct {
	engine.BuiltinPlugin

	cfg        *Config
	vad        Processor
	newVad     func(hopSize int, threshold float32) (Processor, error)
	frameBytes int
	frameBuf   []byte
	sampleBuf  []int16

	preSpeechFrames  int
	postSpeechFrames int
	speechFrames     int
	silenceFrames    int
	speaking         bool
}

func NewPlugin(config *Config) engine.Plugin {
	cfg := normalizeConfig(config)
	return &Plugin{
		cfg:              cfg,
		newVad:           func(hopSize int, threshold float32) (Processor, error) { return NewVad(hopSize, threshold) },
		frameBytes:       cfg.FrameBytes,
		preSpeechFrames:  durationToFrames(cfg.MinSpeechDurationMs),
		postSpeechFrames: durationToFrames(cfg.MinSilenceDurationMs),
	}
}

func (p *Plugin) OnStart(ctx context.Context, _ engine.Flow) error {
	slog.InfoContext(ctx, "ten-vad plugin OnStart")
	vad, err := p.newVad(p.frameBytes/2, p.cfg.Threshold)
	if err != nil {
		return err
	}
	p.vad = vad
	p.sampleBuf = make([]int16, vad.FrameSize())
	return nil
}

func (p *Plugin) OnStop() {
	p.reset()
	if p.vad == nil {
		return
	}
	_ = p.vad.Close()
	p.vad = nil
	p.sampleBuf = nil
}

func (p *Plugin) OnAudio(_ context.Context, flow engine.Flow, audio schema.Audio) {
	p.processAudio(flow, audio.Bytes())
	flow.SendAudio(audio)
}

func (p *Plugin) processAudio(flow engine.Flow, data []byte) {
	offset, size := 0, len(data)

	if len(p.frameBuf) > 0 {
		need := min(p.frameBytes-len(p.frameBuf), size)
		p.frameBuf = append(p.frameBuf, data[:need]...)
		offset += need
		if len(p.frameBuf) == p.frameBytes {
			p.processFrame(flow, p.frameBuf)
			p.frameBuf = p.frameBuf[:0]
		}
	}

	for size-offset >= p.frameBytes {
		end := offset + p.frameBytes
		p.processFrame(flow, data[offset:end])
		offset = end
	}

	if size > offset {
		p.frameBuf = append(p.frameBuf, data[offset:]...)
	}
}

func (p *Plugin) processFrame(flow engine.Flow, frame []byte) {
	byteFrameToInt16(frame, p.sampleBuf)
	_, isSpeech, err := p.vad.Process(p.sampleBuf)
	if err != nil {
		return
	}
	if isSpeech {
		p.speechFrames++
		p.silenceFrames = 0
		if !p.speaking && p.speechFrames >= p.preSpeechFrames {
			p.speaking = true
			flow.SendSignal(schema.NewSignal(schema.SignalVadUserSpeechStart).ReadOnly())
		}
		return
	}

	p.speechFrames = 0
	if !p.speaking {
		return
	}

	p.silenceFrames++
	if p.silenceFrames >= p.postSpeechFrames {
		p.speaking = false
		p.silenceFrames = 0
		flow.SendSignal(schema.NewSignal(schema.SignalVadUserSpeechEnd).ReadOnly())
	}
}

func (p *Plugin) reset() {
	p.frameBuf = p.frameBuf[:0]
	p.speechFrames = 0
	p.silenceFrames = 0
	p.speaking = false
}

func normalizeConfig(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		cfg.Threshold = defaultThreshold
	}
	if cfg.MinSpeechDurationMs <= 0 {
		cfg.MinSpeechDurationMs = defaultMinSpeechMs
	}
	if cfg.MinSilenceDurationMs <= 0 {
		cfg.MinSilenceDurationMs = defaultMinSilenceMs
	}
	if cfg.FrameBytes <= 0 || cfg.FrameBytes%2 != 0 {
		cfg.FrameBytes = defaultFrameBytes
	}
	return cfg
}

func durationToFrames(durationMs int) int {
	frames := (durationMs + defaultFrameDurationMs - 1) / defaultFrameDurationMs
	if frames <= 0 {
		return 1
	}
	return frames
}

func byteFrameToInt16(frame []byte, dst []int16) {
	for i := range dst {
		offset := i * 2
		dst[i] = int16(binary.LittleEndian.Uint16(frame[offset : offset+2]))
	}
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "ten_vad",
		Description: "TEN native VAD for PCM16 16k mono audio with pre/post speech timing",
		Outputs: engine.NewPropertyBuilder().
			AddSignalEvent(schema.SignalVadUserSpeechStart).
			AddSignalEvent(schema.SignalVadUserSpeechEnd).
			Build(),
	}); err != nil {
		panic(err)
	}
}
