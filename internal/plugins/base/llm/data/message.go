package data

const (
	UserMessageRole   = "user"
	SystemMessageRole = "system"
	AiMessageRole     = "assistant"
)

type Message interface {
	Role() string
}

type MessageRole struct {
	MessageRole string `json:"role" bson:"role"`
}

func newMessageRole(t string) MessageRole {
	return MessageRole{MessageRole: t}
}

func (m *MessageRole) Role() string {
	return m.MessageRole
}

type SystemMessage struct {
	MessageRole `bson:",inline"`
	Content     string `json:"content" bson:"content"`
}

func NewSystemMessage(content string) *SystemMessage {
	return &SystemMessage{
		Content:     content,
		MessageRole: newMessageRole(SystemMessageRole),
	}
}

type AssistantMessage struct {
	MessageRole `bson:",inline"`
	Content     string `json:"content,omitempty" bson:"content,omitempty"`
}

func NewAiMessage(content string) *AssistantMessage {
	return &AssistantMessage{
		Content:     content,
		MessageRole: newMessageRole(AiMessageRole),
	}
}

type UserMessage struct {
	MessageRole `bson:",inline"`
	Name        string    `json:"name,omitempty" bson:"name,omitempty"`
	Content     []Content `json:"content" bson:"content"`
}

func NewUserMessage(name string, contents ...Content) *UserMessage {
	if contents == nil {
		contents = make([]Content, 0)
	}
	return &UserMessage{
		MessageRole: newMessageRole(UserMessageRole),
		Name:        name,
		Content:     contents,
	}
}
