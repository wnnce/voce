package engine

import (
	"encoding/json"
)

// WorkflowConfig Represents the dynamic structure of a flow, typically deserialized from JSON.
type WorkflowConfig struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Head    string       `json:"head"`
	Nodes   []NodeConfig `json:"nodes"`
	Edges   []EdgeConfig `json:"edges"`
}

// NodeConfig Represent a specific instance of an Plugin within the Workflow.
type NodeConfig struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Plugin   string          `json:"plugin"`
	Config   json.RawMessage `json:"config"`
	Metadata map[string]any  `json:"metadata"`
}

// EdgeConfig Represents the directed data/control flow between two nodes.
type EdgeConfig struct {
	Source     string    `json:"source"`
	SourcePort int       `json:"source_port"`
	Target     string    `json:"target"`
	Type       EventType `json:"type"`
}
