package schema

const (
	PayloadASRResult = "asr_result"
	PayloadCaption   = "caption"
	PayloadLLMChunk  = "llm_chunk"
)

// Payload is the read-only interface for structured data passed between nodes (e.g. ASR results, LLM chunks).
type Payload interface {
	View
	Mutable() MutablePayload // Returns a writable copy if the current instance is ReadOnly
}

// MutablePayload is the writable interface for payload objects.
// It embeds View directly (NOT Payload), so MutablePayload does NOT satisfy Payload.
// Callers must explicitly call ReadOnly() to get a Payload that can be passed to SendPayload.
type MutablePayload interface {
	View
	Properties
	ReadOnly() Payload
}

type builtinPayload struct {
	builtinProperties
	name string
}

func NewPayload(name string) MutablePayload {
	return &builtinPayload{
		name: name,
		builtinProperties: builtinProperties{
			entries: make([]entry, 0),
		},
	}
}

func (b *builtinPayload) Name() string {
	return b.name
}

func (b *builtinPayload) Mutable() MutablePayload {
	if b.isReadOnly() {
		return &builtinPayload{
			name:              b.name,
			builtinProperties: *b.builtinProperties.Clone(),
		}
	}
	return b
}

func (b *builtinPayload) ReadOnly() Payload {
	b.setReadOnly()
	return b
}
