package metadata

const (
	ContextTraceKey    ContextKeyStr = "trace_id"
	ContextNodeNameKey ContextKeyStr = "node"
)

type ContextKeyStr string

func (c ContextKeyStr) String() string {
	return string(c)
}
