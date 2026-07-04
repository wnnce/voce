package llm

import (
	"github.com/wnnce/voce/internal/schema"
)

type Chunk struct {
	Sentence string `json:"sentence"`
	IsFinal  bool   `json:"is_final"`
}

func (c *Chunk) PackSchema(props schema.Properties) error {
	if err := props.Set("sentence", c.Sentence); err != nil {
		return err
	}
	if err := props.Set("is_final", c.IsFinal); err != nil {
		return err
	}
	return nil
}

func (c *Chunk) UnpackSchema(data schema.ReadOnly) error {
	c.Sentence = schema.GetAs(data, "sentence", "")
	c.IsFinal = schema.GetAs(data, "is_final", false)
	return nil
}
