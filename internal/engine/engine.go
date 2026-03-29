package engine

type EventType byte

const (
	EventSignal EventType = iota + 1
	EventPayload
	EventAudio
	EventVideo
)

type controlType byte

const (
	controlPause controlType = iota + 1
	controlResume
)

const (
	MaxPortCount = 12
)

const (
	AudioFormat     = "pcm"
	AudioSampleRate = 16000
	AudioChannels   = 1
	AudioFrameSize  = AudioSampleRate * AudioChannels * 2 * 10 / 1000
	AudioBufferSize = AudioFrameSize * 10
)

type WorkflowState int32

const (
	WorkflowStatePending WorkflowState = iota
	WorkflowStateStarting
	WorkflowStateRunning
	WorkflowStatePaused
	WorkflowStateStopped
)

type DropStrategy int

const (
	DropNewest DropStrategy = iota
	BlockIfFull
)
