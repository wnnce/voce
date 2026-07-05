package schema

type ResultStatus int

const (
	ResultStatusUnspecified ResultStatus = iota
	ResultStatusOK
	ResultStatusError
	ResultStatusCanceled
)

// Result is an immutable named result object returned by command-like handlers.
// It exposes read-only properties plus a terminal status.
type Result interface {
	View
	Status() ResultStatus
}

// ResultBuilder incrementally constructs a Result and freezes it on Build.
type ResultBuilder interface {
	Set(key string, value any) error
	Pack(src any) error
	Build() Result
}

type builtinResult struct {
	builtinProperties
	name   string
	status ResultStatus
}

type builtinResultBuilder struct {
	result *builtinResult
}

func NewResultBuilder(name string, status ResultStatus) ResultBuilder {
	return &builtinResultBuilder{
		result: &builtinResult{
			name:   name,
			status: status,
			builtinProperties: builtinProperties{
				entries: make([]entry, 0),
			},
		},
	}
}

func (b *builtinResult) Name() string {
	return b.name
}

func (b *builtinResult) Status() ResultStatus {
	return b.status
}

func (b *builtinResultBuilder) Set(key string, value any) error {
	return b.result.Set(key, value)
}

func (b *builtinResultBuilder) Pack(src any) error {
	return b.result.Pack(src)
}

func (b *builtinResultBuilder) Build() Result {
	b.result.setReadOnly()
	return b.result
}
