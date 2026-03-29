package benchmark

import (
	"context"
	"os"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

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
	file *os.File
}

func NewRecorderPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	f, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)
	return &RecorderPlugin{
		file: f,
	}
}

func (e *RecorderPlugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
	if e.file != nil {
		_, _ = e.file.Write(audio.Bytes())
	}
	flow.SendAudio(audio)
}

func (e *RecorderPlugin) OnStop() {
	if e.file != nil {
		_ = e.file.Close()
	}
	e.BuiltinPlugin.OnStop()
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
