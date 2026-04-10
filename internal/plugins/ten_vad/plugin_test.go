package ten_vad

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type fakeProcessor struct {
	frameSize int
	results   []float32
	calls     int
}

func (f *fakeProcessor) Process(_ []int16) (float32, bool, error) {
	val := f.results[f.calls%len(f.results)]
	f.calls++
	return val, val >= 0.5, nil
}

func (f *fakeProcessor) FrameSize() int {
	return f.frameSize
}

func (f *fakeProcessor) Close() error {
	return nil
}

func TestPlugin_OnAudio_BufferAndSpeechTiming(t *testing.T) {
	processor := &fakeProcessor{
		frameSize: 256,
		results:   []float32{0.9, 0.8, 0.1, 0.1},
	}

	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  32,
		MinSilenceDurationMs: 32,
		FrameBytes:           512,
	}).(*Plugin)
	plugin.newVad = func(_ int, _ float32) (Processor, error) {
		return processor, nil
	}

	var signals []string
	tester := engine.NewPluginTester(t, plugin)
	tester.OnSignal(func(_ int, signal schema.Signal) {
		signals = append(signals, signal.Name())
	})

	tester.Start()

	audio1 := schema.NewAudio("input", 16000, 1)
	audio1.SetBytes(make([]byte, 256))
	tester.InjectAudio(audio1.ReadOnly())
	assert.Equal(t, 0, processor.calls)

	audio2 := schema.NewAudio("input", 16000, 1)
	audio2.SetBytes(make([]byte, 256))
	tester.InjectAudio(audio2.ReadOnly())
	assert.Equal(t, 1, processor.calls)
	assert.Empty(t, signals)

	audio3 := schema.NewAudio("input", 16000, 1)
	audio3.SetBytes(make([]byte, 512*3))
	tester.InjectAudio(audio3.ReadOnly())
	assert.Equal(t, 4, processor.calls)
	assert.Equal(t, []string{schema.SignalVadUserSpeechStart, schema.SignalVadUserSpeechEnd}, signals)

	tester.Stop()
}

func TestPlugin_OnAudio_50msChunks(t *testing.T) {
	processor := &fakeProcessor{
		frameSize: 256,
		results:   []float32{0.9, 0.8, 0.1, 0.1, 0.1, 0.1},
	}

	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  32,
		MinSilenceDurationMs: 48,
		FrameBytes:           512,
	}).(*Plugin)
	plugin.newVad = func(_ int, _ float32) (Processor, error) {
		return processor, nil
	}

	var signals []string
	tester := engine.NewPluginTester(t, plugin)
	tester.OnSignal(func(_ int, signal schema.Signal) {
		signals = append(signals, signal.Name())
	})

	tester.Start()

	audio1 := schema.NewAudio("input", 16000, 1)
	audio1.SetBytes(make([]byte, 1600))
	tester.InjectAudio(audio1.ReadOnly())

	audio2 := schema.NewAudio("input", 16000, 1)
	audio2.SetBytes(make([]byte, 1600))
	tester.InjectAudio(audio2.ReadOnly())

	assert.Equal(t, 6, processor.calls)
	assert.Equal(t, []string{schema.SignalVadUserSpeechStart, schema.SignalVadUserSpeechEnd}, signals)

	tester.Stop()
}

func TestPlugin_OnAudio_100msChunks(t *testing.T) {
	processor := &fakeProcessor{
		frameSize: 256,
		results:   []float32{0.9, 0.9, 0.9, 0.9, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
	}

	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  48,
		MinSilenceDurationMs: 80,
		FrameBytes:           512,
	}).(*Plugin)
	plugin.newVad = func(_ int, _ float32) (Processor, error) {
		return processor, nil
	}

	var signals []string
	tester := engine.NewPluginTester(t, plugin)
	tester.OnSignal(func(_ int, signal schema.Signal) {
		signals = append(signals, signal.Name())
	})

	tester.Start()

	audio1 := schema.NewAudio("input", 16000, 1)
	audio1.SetBytes(make([]byte, 3200))
	tester.InjectAudio(audio1.ReadOnly())

	audio2 := schema.NewAudio("input", 16000, 1)
	audio2.SetBytes(make([]byte, 3200))
	tester.InjectAudio(audio2.ReadOnly())

	assert.Equal(t, 12, processor.calls)
	assert.Equal(t, []string{schema.SignalVadUserSpeechStart, schema.SignalVadUserSpeechEnd}, signals)

	tester.Stop()
}

func BenchmarkPlugin_OnAudio_50ms(b *testing.B) {
	benchmarkPluginOnAudio(b, 1600)
}

func BenchmarkPlugin_OnAudio_100ms(b *testing.B) {
	benchmarkPluginOnAudio(b, 3200)
}

func BenchmarkPlugin_InjectAudio_50ms(b *testing.B) {
	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  60,
		MinSilenceDurationMs: 100,
		FrameBytes:           512,
	}).(*Plugin)

	flow := &engine.MockFlow{}
	audio := schema.NewAudio("input", 16000, 1)
	audio.SetBytes(make([]byte, 1600))
	readonly := audio.ReadOnly()

	if err := plugin.OnStart(nil, flow); err != nil {
		b.Fatalf("plugin start failed: %v", err)
	}
	defer plugin.OnStop()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		plugin.OnAudio(nil, flow, readonly)
	}
}

func BenchmarkPlugin_OnAudio_50ms_FakeVAD(b *testing.B) {
	benchmarkPluginOnAudioFake(b, 1600, []float32{0.9, 0.8, 0.1, 0.1, 0.1})
}

func BenchmarkPlugin_OnAudio_100ms_FakeVAD(b *testing.B) {
	benchmarkPluginOnAudioFake(b, 3200, []float32{0.9, 0.9, 0.9, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1})
}

func BenchmarkVadProcess_Frame(b *testing.B) {
	vad, err := NewVad(256, 0.5)
	if err != nil {
		b.Fatalf("vad init failed: %v", err)
	}
	defer func() { _ = vad.Close() }()

	frame := make([]int16, 256)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, err := vad.Process(frame)
		if err != nil {
			b.Fatalf("vad process failed: %v", err)
		}
	}
}

func benchmarkPluginOnAudio(b *testing.B, audioBytes int) {
	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  60,
		MinSilenceDurationMs: 100,
		FrameBytes:           512,
	}).(*Plugin)

	flow := &engine.MockFlow{}
	audio := schema.NewAudio("input", 16000, 1)
	audio.SetBytes(make([]byte, audioBytes))
	readonly := audio.ReadOnly()

	if err := plugin.OnStart(nil, flow); err != nil {
		b.Fatalf("plugin start failed: %v", err)
	}
	defer plugin.OnStop()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		plugin.OnAudio(nil, flow, readonly)
	}
}

func benchmarkPluginOnAudioFake(b *testing.B, audioBytes int, results []float32) {
	processor := &fakeProcessor{
		frameSize: 256,
		results:   results,
	}

	plugin := NewPlugin(&Config{
		Threshold:            0.5,
		MinSpeechDurationMs:  60,
		MinSilenceDurationMs: 100,
		FrameBytes:           512,
	}).(*Plugin)
	plugin.newVad = func(_ int, _ float32) (Processor, error) {
		return processor, nil
	}

	flow := &engine.MockFlow{}
	audio := schema.NewAudio("input", 16000, 1)
	audio.SetBytes(make([]byte, audioBytes))
	readonly := audio.ReadOnly()

	if err := plugin.OnStart(nil, flow); err != nil {
		b.Fatalf("plugin start failed: %v", err)
	}
	defer plugin.OnStop()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		plugin.OnAudio(nil, flow, readonly)
	}
}
