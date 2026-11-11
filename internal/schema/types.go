package schema

import (
	"github.com/bytedance/sonic"
)

// Role constants for captions and dialog participants.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type BaseSchema struct {
	TurnID   uint64         `json:"turn_id,omitempty" schema:"turn_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" schema:"metadata,omitempty"`
}

func (b *BaseSchema) UnpackSchema(data ReadOnly) error {
	if val, ok := data.Get("turn_id"); ok {
		switch v := val.(type) {
		case uint64:
			b.TurnID = v
		case int:
			b.TurnID = uint64(v)
		case int64:
			b.TurnID = uint64(v)
		case float64:
			b.TurnID = uint64(v)
		}
	} else {
		b.TurnID = 0
	}
	if val, ok := data.Get("metadata"); ok {
		switch v := val.(type) {
		case map[string]any:
			b.Metadata = v
		case []byte:
			var m map[string]any
			if err := sonic.Unmarshal(v, &m); err == nil {
				b.Metadata = m
			}
		}
	} else {
		b.Metadata = nil
	}
	return nil
}

func (b *BaseSchema) PackSchema(props Properties) error {
	if b.TurnID > 0 {
		if err := props.Set("turn_id", b.TurnID); err != nil {
			return err
		}
	}
	if len(b.Metadata) > 0 {
		if err := props.Set("metadata", b.Metadata); err != nil {
			return err
		}
	}
	return nil
}

// Transcription represents an ASR speech recognition result exchanged across nodes.
type Transcription struct {
	BaseSchema
	Text         string `json:"text" schema:"text"`
	Final        bool   `json:"is_final" schema:"is_final"`
	Stable       bool   `json:"is_stable" schema:"is_stable"`
	Speaker      string `json:"speaker,omitempty" schema:"speaker"`
	Emotion      string `json:"emotion,omitempty" schema:"emotion"`
	LanguageCode string `json:"language_code,omitempty" schema:"language_code"`
}

func (u *Transcription) UnpackSchema(data ReadOnly) error {
	if err := u.BaseSchema.UnpackSchema(data); err != nil {
		return err
	}
	u.Text = GetAs(data, "text", "")
	u.Final = GetAs(data, "is_final", false)
	u.Stable = GetAs(data, "is_stable", false)
	u.Speaker = GetAs(data, "speaker", "")
	u.Emotion = GetAs(data, "emotion", "")
	u.LanguageCode = GetAs(data, "language_code", "")
	return nil
}

func (u *Transcription) PackSchema(props Properties) error {
	if err := u.BaseSchema.PackSchema(props); err != nil {
		return err
	}
	if err := props.Set("text", u.Text); err != nil {
		return err
	}
	if err := props.Set("is_final", u.Final); err != nil {
		return err
	}
	if err := props.Set("is_stable", u.Stable); err != nil {
		return err
	}
	if err := props.Set("speaker", u.Speaker); err != nil {
		return err
	}
	if u.Emotion != "" {
		if err := props.Set("emotion", u.Emotion); err != nil {
			return err
		}
	}
	if u.LanguageCode != "" {
		if err := props.Set("language_code", u.LanguageCode); err != nil {
			return err
		}
	}
	return nil
}

// Caption represents a subtitle item dispatched to clients or downstream sinks.
type Caption struct {
	BaseSchema
	Text        string `json:"text" schema:"text"`
	Role        string `json:"role" schema:"role"`
	IsFinal     bool   `json:"is_final" schema:"is_final"`
	Speaker     string `json:"speaker,omitempty" schema:"speaker"`
	Translation string `json:"translation,omitempty" schema:"translation"`
}

func (c *Caption) UnpackSchema(data ReadOnly) error {
	if err := c.BaseSchema.UnpackSchema(data); err != nil {
		return err
	}
	c.Text = GetAs(data, "text", "")
	c.Role = GetAs(data, "role", "")
	c.IsFinal = GetAs(data, "is_final", false)
	c.Speaker = GetAs(data, "speaker", "")
	c.Translation = GetAs(data, "translation", "")
	return nil
}

func (c *Caption) PackSchema(props Properties) error {
	if err := c.BaseSchema.PackSchema(props); err != nil {
		return err
	}
	if err := props.Set("text", c.Text); err != nil {
		return err
	}
	if err := props.Set("role", c.Role); err != nil {
		return err
	}
	if err := props.Set("is_final", c.IsFinal); err != nil {
		return err
	}
	if c.Speaker != "" {
		if err := props.Set("speaker", c.Speaker); err != nil {
			return err
		}
	}
	if c.Translation != "" {
		if err := props.Set("translation", c.Translation); err != nil {
			return err
		}
	}
	return nil
}

// LLMChunk represents a streaming text chunk from an LLM.
type LLMChunk struct {
	BaseSchema
	Sentence string `json:"sentence" schema:"sentence"`
	IsFinal  bool   `json:"is_final" schema:"is_final"`
}

func (c *LLMChunk) UnpackSchema(data ReadOnly) error {
	if err := c.BaseSchema.UnpackSchema(data); err != nil {
		return err
	}
	c.Sentence = GetAs(data, "sentence", "")
	c.IsFinal = GetAs(data, "is_final", false)
	return nil
}

func (c *LLMChunk) PackSchema(props Properties) error {
	if err := c.BaseSchema.PackSchema(props); err != nil {
		return err
	}
	if err := props.Set("sentence", c.Sentence); err != nil {
		return err
	}
	if err := props.Set("is_final", c.IsFinal); err != nil {
		return err
	}
	return nil
}
