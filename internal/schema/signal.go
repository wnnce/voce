package schema

const (
	SignalInterrupter        = "interrupter"
	SignalAgentSpeechStart   = "agent_speech_start"
	SignalAgentSpeechEnd     = "agent_speech_end"
	SignalUserSpeechStart    = "user_speech_start"
	SignalUserSpeechEnd      = "user_speech_end"
	SignalVadUserSpeechStart = "vad_user_speech_start"
	SignalVadUserSpeechEnd   = "vad_user_speech_end"
)

// Signal is the read-only interface for control/event objects passed between nodes.
type Signal interface {
	View
	Mutable() MutableSignal
}

// MutableSignal is the writable interface for signal objects.
// It embeds View directly (NOT Signal), so MutableSignal does NOT satisfy Signal.
// Callers must explicitly call Seal() to get a Signal that can be passed to SendSignal.
type MutableSignal interface {
	View
	Properties
	ReadOnly() Signal
}

type builtinSignal struct {
	builtinProperties
	name string
}

func NewSignal(name string) MutableSignal {
	return &builtinSignal{
		builtinProperties: builtinProperties{
			entries: make([]entry, 0),
		},
		name: name,
	}
}

func (b *builtinSignal) Mutable() MutableSignal {
	if b.isReadOnly() {
		return &builtinSignal{
			builtinProperties: *b.builtinProperties.Clone(),
			name:              b.name,
		}
	}
	return b
}

func (b *builtinSignal) Name() string {
	return b.name
}

func (b *builtinSignal) ReadOnly() Signal {
	b.setReadOnly()
	return b
}
