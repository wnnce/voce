package benchmark

import (
	"bufio"
	"context"
	"os"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

const recorderBufferSize = 64 * 1024

// ForwarderPlugin is a pure pass-through extension
type ForwarderPlugin struct {
	engine.BuiltinPlugin
}

func NewForwarderPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &ForwarderPlugin{}
}

// RecorderPlugin simulates a write operation before forwarding
type RecorderPlugin struct {
	engine.BuiltinPlugin
	file   *os.File
	writer *bufio.Writer
}

func NewRecorderPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)
	if err != nil {
		return &RecorderPlugin{}
	}
	return &RecorderPlugin{
		file:   f,
		writer: bufio.NewWriterSize(f, recorderBufferSize),
	}
}

func (e *RecorderPlugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
	if e.writer != nil {
		_, _ = e.writer.Write(audio.Bytes())
	}
	flow.SendAudio(audio)
}

func (e *RecorderPlugin) OnStop() {
	if e.writer != nil {
		_ = e.writer.Flush()
	}
	if e.file != nil {
		_ = e.file.Close()
	}
}

func init() {
	if err := engine.RegisterPlugin(NewForwarderPlugin, engine.PluginMetadata{
		Name:        "benchmark_forwarder",
		Description: "A pure pass-through extension for benchmarking that forwards everything directly.",
	}); err != nil {
		panic(err)
	}

	if err := engine.RegisterPlugin(NewRecorderPlugin, engine.PluginMetadata{
		Name:        "benchmark_recorder",
		Description: "Simulates a file writing operation (I/O) to /dev/null before forwarding the audio data.",
	}); err != nil {
		panic(err)
	}
}
