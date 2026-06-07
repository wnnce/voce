package engine

type EventType byte

const (
	EventSignal EventType = iota + 1
	EventPayload
	EventAudio
	EventVideo
)

func (e EventType) String() string {
	switch e {
	case EventSignal:
		return "signal"
	case EventPayload:
		return "payload"
	case EventAudio:
		return "audio"
	case EventVideo:
		return "video"
	default:
		return "unknown"
	}
}

type controlType byte

const (
	controlPause controlType = iota + 1
	controlResume
)

func (c controlType) String() string {
	switch c {
	case controlPause:
		return "pause"
	case controlResume:
		return "resume"
	default:
		return "unknown"
	}
}

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

func (w WorkflowState) String() string {
	switch w {
	case WorkflowStatePending:
		return "pending"
	case WorkflowStateStarting:
		return "starting"
	case WorkflowStateRunning:
		return "running"
	case WorkflowStatePaused:
		return "paused"
	case WorkflowStateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

type DropStrategy int

const (
	DropNewest DropStrategy = iota
	BlockIfFull
	DropOldest
)

func (d DropStrategy) String() string {
	switch d {
	case DropNewest:
		return "drop_newest"
	case BlockIfFull:
		return "block_if_full"
	case DropOldest:
		return "drop_oldest"
	default:
		return "unknown"
	}
}
