package metadata

const (
	ContextTraceKey ContextKeyStr = "trace_id"
)

type ContextKeyStr string

func (c ContextKeyStr) String() string {
	return string(c)
}
