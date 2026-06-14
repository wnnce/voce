package mllm

import (
	"fmt"
	"log/slog"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
)

const (
	QwenRealtimeEventSessionUpdate          = "session.update"
	QwenRealtimeEventResponseCreate         = "response.create"
	QwenRealtimeEventResponseCancel         = "response.cancel"
	QwenRealtimeEventInputAudioBufferAppend = "input_audio_buffer.append"
	QwenRealtimeEventInputAudioBufferCommit = "input_audio_buffer.commit"
	QwenRealtimeEventInputAudioBufferClear  = "input_audio_buffer.clear"
	QwenRealtimeEventInputImageBufferAppend = "input_image_buffer.append"
	QwenRealtimeEventConversationItemCreate = "conversation.item.create"
)

const (
	QwenRealtimeEventError                              = "error"
	QwenRealtimeEventSessionCreated                     = "session.created"
	QwenRealtimeEventSessionUpdated                     = "session.updated"
	QwenRealtimeEventInputAudioBufferSpeechStarted      = "input_audio_buffer.speech_started"
	QwenRealtimeEventInputAudioBufferSpeechStopped      = "input_audio_buffer.speech_stopped"
	QwenRealtimeEventInputAudioBufferCommitted          = "input_audio_buffer.committed"
	QwenRealtimeEventInputAudioBufferCleared            = "input_audio_buffer.cleared"
	QwenRealtimeEventConversationItemCreated            = "conversation.item.created"
	QwenRealtimeEventInputAudioTranscriptionDelta       = "conversation.item.input_audio_transcription.delta"
	QwenRealtimeEventInputAudioTranscriptionCompleted   = "conversation.item.input_audio_transcription.completed"
	QwenRealtimeEventInputAudioTranscriptionFailed      = "conversation.item.input_audio_transcription.failed"
	QwenRealtimeEventResponseCreated                    = "response.created"
	QwenRealtimeEventResponseDone                       = "response.done"
	QwenRealtimeEventResponseTextDelta                  = "response.text.delta"
	QwenRealtimeEventResponseTextDone                   = "response.text.done"
	QwenRealtimeEventResponseAudioDelta                 = "response.audio.delta"
	QwenRealtimeEventResponseAudioDone                  = "response.audio.done"
	QwenRealtimeEventResponseAudioTranscriptDelta       = "response.audio_transcript.delta"
	QwenRealtimeEventResponseAudioTranscriptDone        = "response.audio_transcript.done"
	QwenRealtimeEventResponseFunctionCallArgumentsDelta = "response.function_call_arguments.delta"
	QwenRealtimeEventResponseFunctionCallArgumentsDone  = "response.function_call_arguments.done"
	QwenRealtimeEventResponseOutputItemAdded            = "response.output_item.added"
	QwenRealtimeEventResponseOutputItemDone             = "response.output_item.done"
	QwenRealtimeEventResponseContentPartAdded           = "response.content_part.added"
	QwenRealtimeEventResponseContentPartDone            = "response.content_part.done"
)

const (
	QwenRealtimeModalityText  = "text"
	QwenRealtimeModalityAudio = "audio"

	QwenRealtimeAudioFormatPCM = "pcm"

	QwenRealtimeTurnDetectionServerVAD   = "server_vad"
	QwenRealtimeTurnDetectionSemanticVAD = "semantic_vad"

	QwenRealtimeToolTypeFunction = "function"

	QwenRealtimeConversationItemFunctionCallOutput = "function_call_output"

	QwenRealtimeSessionObject  = "realtime.session"
	QwenRealtimeItemObject     = "realtime.item"
	QwenRealtimeResponseObject = "realtime.response"

	QwenRealtimeItemTypeMessage      = "message"
	QwenRealtimeItemTypeFunctionCall = "function_call"

	QwenRealtimeContentTypeInputAudio = "input_audio"
	QwenRealtimeContentTypeText       = "text"
	QwenRealtimeContentTypeAudio      = "audio"

	QwenRealtimeRoleAssistant = "assistant"
	QwenRealtimeRoleUser      = "user"

	QwenRealtimeStatusInProgress = "in_progress"
	QwenRealtimeStatusCompleted  = "completed"
	QwenRealtimeStatusFailed     = "failed"
	QwenRealtimeStatusIncomplete = "incomplete"
)

type QwenRealtimeSessionUpdateEvent struct {
	qwenRealtimeClientMessageHeader
	Session *QwenRealtimeSessionOptions `json:"session,omitempty"`
}

func NewQwenRealtimeSessionUpdateEvent(eventID string, session *QwenRealtimeSessionOptions) QwenRealtimeSessionUpdateEvent {
	return QwenRealtimeSessionUpdateEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventSessionUpdate),
		Session:                         session,
	}
}

type QwenRealtimeSessionOptions struct {
	ID                      string                               `json:"id,omitempty"`
	Object                  string                               `json:"object,omitempty"`
	Model                   string                               `json:"model,omitempty"`
	Modalities              []string                             `json:"modalities,omitempty"`
	Voice                   string                               `json:"voice,omitempty"`
	InputAudioFormat        string                               `json:"input_audio_format,omitempty"`
	OutputAudioFormat       string                               `json:"output_audio_format,omitempty"`
	InputAudioTranscription *QwenRealtimeInputAudioTranscription `json:"input_audio_transcription,omitempty"`
	SmoothOutput            *bool                                `json:"smooth_output,omitempty"`
	Instructions            string                               `json:"instructions,omitempty"`
	TurnDetection           *QwenRealtimeTurnDetection           `json:"turn_detection,omitempty"`
	EnableSearch            *bool                                `json:"enable_search,omitempty"`
	SearchOptions           *QwenRealtimeSearchOptions           `json:"search_options,omitempty"`
	Tools                   []QwenRealtimeTool                   `json:"tools,omitempty"`
	Seed                    *int                                 `json:"seed,omitempty"`
	MaxTokens               *int                                 `json:"max_tokens,omitempty"`
	MaxResponseOutputToken  string                               `json:"max_response_output_token,omitempty"`
	RepetitionPenalty       *float64                             `json:"repetition_penalty,omitempty"`
	PresencePenalty         *float64                             `json:"presence_penalty,omitempty"`
	TopK                    *int                                 `json:"top_k,omitempty"`
	TopP                    *float64                             `json:"top_p,omitempty"`
	Temperature             *float64                             `json:"temperature,omitempty"`
}

type QwenRealtimeInputAudioTranscription struct {
	Model string `json:"model,omitempty"`
}

type QwenRealtimeTurnDetection struct {
	Type              string   `json:"type,omitempty"`
	Threshold         *float64 `json:"threshold,omitempty"`
	PrefixPaddingMS   *int     `json:"prefix_padding_ms,omitempty"`
	SilenceDurationMS *int     `json:"silence_duration_ms,omitempty"`
	IdleTimeoutMS     *int     `json:"idle_timeout_ms,omitempty"`
	CreateResponse    *bool    `json:"create_response,omitempty"`
	InterruptResponse *bool    `json:"interrupt_response,omitempty"`
}

type QwenRealtimeSearchOptions struct {
	EnableSource *bool `json:"enable_source,omitempty"`
}

type QwenRealtimeTool struct {
	Type     string                   `json:"type"`
	Function QwenRealtimeToolFunction `json:"function"`
}

type QwenRealtimeToolFunction struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description,omitempty"`
	Parameters  *QwenRealtimeToolParameters `json:"parameters,omitempty"`
}

type QwenRealtimeToolParameters struct {
	Type       string                                     `json:"type"`
	Properties map[string]QwenRealtimeToolParameterSchema `json:"properties,omitempty"`
	Required   []string                                   `json:"required,omitempty"`
}

type QwenRealtimeToolParameterSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type QwenRealtimeClientMessage interface {
	clientMessage()
}

type QwenRealtimeServerMessage interface {
	serverMessage()
}

type qwenRealtimeClientMessageHeader struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

func (qwenRealtimeClientMessageHeader) clientMessage() {}

type qwenRealtimeServerMessageHeader struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

func (qwenRealtimeServerMessageHeader) serverMessage() {}

func newQwenRealtimeMessageHeader(eventID, eventType string) qwenRealtimeClientMessageHeader {
	return qwenRealtimeClientMessageHeader{
		EventID: eventID,
		Type:    eventType,
	}
}

type QwenRealtimeResponseCreateEvent struct {
	qwenRealtimeClientMessageHeader
}

func NewQwenRealtimeResponseCreateEvent(eventID string) QwenRealtimeResponseCreateEvent {
	return QwenRealtimeResponseCreateEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventResponseCreate),
	}
}

type QwenRealtimeResponseCancelEvent struct {
	qwenRealtimeClientMessageHeader
}

func NewQwenRealtimeResponseCancelEvent(eventID string) QwenRealtimeResponseCancelEvent {
	return QwenRealtimeResponseCancelEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventResponseCancel),
	}
}

type QwenRealtimeInputAudioBufferAppendEvent struct {
	qwenRealtimeClientMessageHeader
	Audio string `json:"audio"`
}

func NewQwenRealtimeInputAudioBufferAppendEvent(eventID, audio string) QwenRealtimeInputAudioBufferAppendEvent {
	return QwenRealtimeInputAudioBufferAppendEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventInputAudioBufferAppend),
		Audio:                           audio,
	}
}

type QwenRealtimeInputAudioBufferCommitEvent struct {
	qwenRealtimeClientMessageHeader
}

func NewQwenRealtimeInputAudioBufferCommitEvent(eventID string) QwenRealtimeInputAudioBufferCommitEvent {
	return QwenRealtimeInputAudioBufferCommitEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventInputAudioBufferCommit),
	}
}

type QwenRealtimeInputAudioBufferClearEvent struct {
	qwenRealtimeClientMessageHeader
}

func NewQwenRealtimeInputAudioBufferClearEvent(eventID string) QwenRealtimeInputAudioBufferClearEvent {
	return QwenRealtimeInputAudioBufferClearEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventInputAudioBufferClear),
	}
}

type QwenRealtimeInputImageBufferAppendEvent struct {
	qwenRealtimeClientMessageHeader
	Image string `json:"image"`
}

func NewQwenRealtimeInputImageBufferAppendEvent(eventID, image string) QwenRealtimeInputImageBufferAppendEvent {
	return QwenRealtimeInputImageBufferAppendEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventInputImageBufferAppend),
		Image:                           image,
	}
}

type QwenRealtimeConversationItemCreateEvent struct {
	qwenRealtimeClientMessageHeader
	Item QwenRealtimeConversationItemOutput `json:"item"`
}

func NewQwenRealtimeConversationItemCreateEvent(
	eventID string,
	item QwenRealtimeConversationItemOutput,
) QwenRealtimeConversationItemCreateEvent {
	return QwenRealtimeConversationItemCreateEvent{
		qwenRealtimeClientMessageHeader: newQwenRealtimeMessageHeader(eventID, QwenRealtimeEventConversationItemCreate),
		Item:                            item,
	}
}

type QwenRealtimeConversationItemOutput struct {
	ID     string `json:"id,omitempty"`
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type QwenRealtimeErrorEvent struct {
	qwenRealtimeServerMessageHeader
	Error QwenRealtimeError `json:"error"`
}

type QwenRealtimeError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Param   string `json:"param,omitempty"`
}

type QwenRealtimeSessionCreatedEvent struct {
	qwenRealtimeServerMessageHeader
	Session QwenRealtimeSessionOptions `json:"session"`
}

type QwenRealtimeSessionUpdatedEvent struct {
	qwenRealtimeServerMessageHeader
	Session QwenRealtimeSessionOptions `json:"session"`
}

type QwenRealtimeInputAudioBufferSpeechStartedEvent struct {
	qwenRealtimeServerMessageHeader
	AudioStartMS int    `json:"audio_start_ms"`
	ItemID       string `json:"item_id"`
}

type QwenRealtimeInputAudioBufferSpeechStoppedEvent struct {
	qwenRealtimeServerMessageHeader
	AudioEndMS int    `json:"audio_end_ms"`
	ItemID     string `json:"item_id"`
}

type QwenRealtimeInputAudioBufferCommittedEvent struct {
	qwenRealtimeServerMessageHeader
	ItemID string `json:"item_id"`
}

type QwenRealtimeInputAudioBufferClearedEvent struct {
	qwenRealtimeServerMessageHeader
}

type QwenRealtimeConversationItemCreatedEvent struct {
	qwenRealtimeServerMessageHeader
	Item QwenRealtimeConversationItem `json:"item"`
}

type QwenRealtimeConversationItem struct {
	ID        string                    `json:"id,omitempty"`
	Object    string                    `json:"object,omitempty"`
	Type      string                    `json:"type,omitempty"`
	Status    string                    `json:"status,omitempty"`
	Role      string                    `json:"role,omitempty"`
	Content   []QwenRealtimeContentPart `json:"content,omitempty"`
	Name      string                    `json:"name,omitempty"`
	CallID    string                    `json:"call_id,omitempty"`
	Arguments string                    `json:"arguments,omitempty"`
}

type QwenRealtimeContentPart struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

type QwenRealtimeInputAudioTranscriptionDeltaEvent struct {
	qwenRealtimeServerMessageHeader
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
	Stash        string `json:"stash"`
	Language     string `json:"language,omitempty"`
	Emotion      string `json:"emotion,omitempty"`
	Obfuscation  string `json:"obfuscation,omitempty"`
}

type QwenRealtimeInputAudioTranscriptionCompletedEvent struct {
	qwenRealtimeServerMessageHeader
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

type QwenRealtimeInputAudioTranscriptionFailedEvent struct {
	qwenRealtimeServerMessageHeader
	ItemID       string            `json:"item_id"`
	ContentIndex int               `json:"content_index"`
	Error        QwenRealtimeError `json:"error"`
}

type QwenRealtimeResponseCreatedEvent struct {
	qwenRealtimeServerMessageHeader
	Response QwenRealtimeResponse `json:"response"`
}

type QwenRealtimeResponseDoneEvent struct {
	qwenRealtimeServerMessageHeader
	Response QwenRealtimeResponse `json:"response"`
}

type QwenRealtimeResponse struct {
	ID                string                         `json:"id,omitempty"`
	Object            string                         `json:"object,omitempty"`
	ConversationID    string                         `json:"conversation_id,omitempty"`
	Status            string                         `json:"status,omitempty"`
	Modalities        []string                       `json:"modalities,omitempty"`
	Voice             string                         `json:"voice,omitempty"`
	OutputAudioFormat string                         `json:"output_audio_format,omitempty"`
	Output            []QwenRealtimeConversationItem `json:"output,omitempty"`
	Usage             *QwenRealtimeUsage             `json:"usage,omitempty"`
}

type QwenRealtimeUsage struct {
	TotalTokens         int                           `json:"total_tokens,omitempty"`
	InputTokens         int                           `json:"input_tokens,omitempty"`
	OutputTokens        int                           `json:"output_tokens,omitempty"`
	InputTokensDetails  QwenRealtimeTokenUsageDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails QwenRealtimeTokenUsageDetails `json:"output_tokens_details,omitempty"`
	Plugins             *QwenRealtimePluginUsage      `json:"plugins,omitempty"`
}

type QwenRealtimeTokenUsageDetails struct {
	TextTokens  int `json:"text_tokens,omitempty"`
	AudioTokens int `json:"audio_tokens,omitempty"`
}

type QwenRealtimePluginUsage struct {
	Search *QwenRealtimeSearchUsage `json:"search,omitempty"`
}

type QwenRealtimeSearchUsage struct {
	Count    int    `json:"count,omitempty"`
	Strategy string `json:"strategy,omitempty"`
}

type QwenRealtimeResponseTextDeltaEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type QwenRealtimeResponseTextDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

type QwenRealtimeResponseAudioDeltaEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        []byte `json:"delta"`
}

type QwenRealtimeResponseAudioDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
}

type QwenRealtimeResponseAudioTranscriptDeltaEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type QwenRealtimeResponseAudioTranscriptDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

type QwenRealtimeResponseFunctionCallArgumentsDeltaEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

type QwenRealtimeResponseFunctionCallArgumentsDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
}

type QwenRealtimeResponseOutputItemAddedEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID  string                       `json:"response_id"`
	OutputIndex int                          `json:"output_index"`
	Item        QwenRealtimeConversationItem `json:"item"`
}

type QwenRealtimeResponseOutputItemDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID  string                       `json:"response_id"`
	OutputIndex int                          `json:"output_index"`
	Item        QwenRealtimeConversationItem `json:"item"`
}

type QwenRealtimeResponseContentPartAddedEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string                  `json:"response_id"`
	ItemID       string                  `json:"item_id"`
	OutputIndex  int                     `json:"output_index"`
	ContentIndex int                     `json:"content_index"`
	Part         QwenRealtimeContentPart `json:"part"`
}

type QwenRealtimeResponseContentPartDoneEvent struct {
	qwenRealtimeServerMessageHeader
	ResponseID   string                  `json:"response_id"`
	ItemID       string                  `json:"item_id"`
	OutputIndex  int                     `json:"output_index"`
	ContentIndex int                     `json:"content_index"`
	Part         QwenRealtimeContentPart `json:"part"`
}

func (q *QwenRealtimeProvider) OnOpen(socket *gws.Conn) {
	q.socket = socket
	q.connected.Store(true)
	slog.InfoContext(q.ctx, "qwen realtime connected")

	sessionOpts := q.cfg.Session
	if len(sessionOpts.Modalities) == 0 {
		sessionOpts.Modalities = []string{QwenRealtimeModalityAudio, QwenRealtimeModalityText}
	}
	if sessionOpts.InputAudioFormat == "" {
		sessionOpts.InputAudioFormat = QwenRealtimeAudioFormatPCM
	}
	if sessionOpts.OutputAudioFormat == "" {
		sessionOpts.OutputAudioFormat = QwenRealtimeAudioFormatPCM
	}
	if sessionOpts.TurnDetection == nil {
		sessionOpts.TurnDetection = &QwenRealtimeTurnDetection{
			Type: QwenRealtimeTurnDetectionServerVAD,
		}
	}
	event := NewQwenRealtimeSessionUpdateEvent("", &sessionOpts)
	if err := q.send(event); err != nil {
		slog.ErrorContext(q.ctx, "qwen realtime send session.update failed", "error", err)
	}
}

func (q *QwenRealtimeProvider) OnClose(socket *gws.Conn, err error) {
	q.connected.Store(false)
	q.socket = nil
	slog.InfoContext(q.ctx, "qwen realtime disconnected", "error", err)
}

//nolint:gocyclo,funlen // Qwen Realtime has many server event variants that are intentionally dispatched here.
func (q *QwenRealtimeProvider) OnMessage(socket *gws.Conn, msg *gws.Message) {
	defer func() {
		_ = msg.Close()
	}()
	if msg.Opcode != gws.OpcodeText {
		return
	}
	payload := msg.Bytes()
	message, err := q.decodeQwenRealtimeServerEvent(payload)
	if err != nil {
		slog.ErrorContext(q.ctx, "qwen realtime decode server event failed",
			slog.Any("error", err),
			slog.Int("payload_bytes", len(payload)),
		)
		return
	}
	switch event := message.(type) {
	case *QwenRealtimeErrorEvent:
		slog.ErrorContext(q.ctx, "qwen-realtime received error",
			slog.String("event_id", event.EventID),
			slog.String("type", event.Error.Type),
			slog.String("code", event.Error.Code),
			slog.String("message", event.Error.Message),
			slog.String("param", event.Error.Param),
		)
		q.listener.OnError(event.Error.Code, event.Error.Message)
	case *QwenRealtimeSessionCreatedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received session created",
			slog.String("event_id", event.EventID),
			slog.String("session_id", event.Session.ID),
			slog.String("model", event.Session.Model),
			slog.String("voice", event.Session.Voice),
			slog.Any("modalities", event.Session.Modalities),
		)
	case *QwenRealtimeSessionUpdatedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received session updated",
			slog.String("event_id", event.EventID),
			slog.String("session_id", event.Session.ID),
			slog.String("model", event.Session.Model),
			slog.String("voice", event.Session.Voice),
			slog.Any("modalities", event.Session.Modalities),
		)
	case *QwenRealtimeInputAudioBufferSpeechStartedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received speech started",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
			slog.Int("audio_start_ms", event.AudioStartMS),
		)
		q.state.Store(stateUserSpeaking)
		q.itemId = event.ItemID
		q.listener.OnSpeechStarted()
	case *QwenRealtimeInputAudioBufferSpeechStoppedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received speech stopped",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
			slog.Int("audio_end_ms", event.AudioEndMS),
		)
		q.listener.OnSpeechStopped()
	case *QwenRealtimeInputAudioBufferCommittedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received audio buffer committed",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
		)
	case *QwenRealtimeInputAudioBufferClearedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received audio buffer cleared",
			slog.String("event_id", event.EventID),
		)
	case *QwenRealtimeConversationItemCreatedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received item created",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.Item.ID),
			slog.String("item_type", event.Item.Type),
			slog.String("status", event.Item.Status),
			slog.String("role", event.Item.Role),
			slog.String("call_id", event.Item.CallID),
			slog.String("name", event.Item.Name),
		)
		q.itemId = event.Item.ID
		q.state.Store(stateWaiting)
	case *QwenRealtimeInputAudioTranscriptionDeltaEvent:
		slog.DebugContext(q.ctx, "qwen-realtime received transcription delta",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("text_len", len(event.Text)),
			slog.Int("stash_len", len(event.Stash)),
			slog.String("language", event.Language),
			slog.String("emotion", event.Emotion),
		)
		q.listener.OnInputTranscriptionDelta(event.Text + event.Stash)
	case *QwenRealtimeInputAudioTranscriptionCompletedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received transcription completed",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("transcript_len", len(event.Transcript)),
		)
		q.listener.OnInputTranscriptionDone(event.Transcript)
	case *QwenRealtimeInputAudioTranscriptionFailedEvent:
		slog.ErrorContext(q.ctx, "qwen-realtime received transcription failed",
			slog.String("event_id", event.EventID),
			slog.String("item_id", event.ItemID),
			slog.Int("content_index", event.ContentIndex),
			slog.String("type", event.Error.Type),
			slog.String("code", event.Error.Code),
			slog.String("message", event.Error.Message),
			slog.String("param", event.Error.Param),
		)
	case *QwenRealtimeResponseCreatedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response created",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.Response.ID),
			slog.String("conversation_id", event.Response.ConversationID),
			slog.String("status", event.Response.Status),
			slog.String("voice", event.Response.Voice),
			slog.Any("modalities", event.Response.Modalities),
		)
	case *QwenRealtimeResponseDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.Response.ID),
			slog.String("conversation_id", event.Response.ConversationID),
			slog.String("status", event.Response.Status),
			slog.Int("output_count", len(event.Response.Output)),
			slog.Int("total_tokens", qwenRealtimeTotalTokens(event.Response.Usage)),
			slog.Int("input_tokens", qwenRealtimeInputTokens(event.Response.Usage)),
			slog.Int("output_tokens", qwenRealtimeOutputTokens(event.Response.Usage)),
		)
	case *QwenRealtimeResponseTextDeltaEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response text delta",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("delta_len", len(event.Delta)),
		)
		q.listener.OnTranscriptDelta(event.Delta)
	case *QwenRealtimeResponseTextDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response text done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("text_len", len(event.Text)),
		)
		q.listener.OnTranscriptDone(event.Text)
	case *QwenRealtimeResponseAudioDeltaEvent:
		slog.DebugContext(q.ctx, "qwen-realtime received response audio delta",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("audio_bytes", len(event.Delta)),
		)
		q.state.Store(stateAgentSpeaking)
		q.listener.OnAudioDelta(event.Delta)
	case *QwenRealtimeResponseAudioDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response audio done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
		)
		q.state.Store(stateListen)
		q.listener.OnAudioDone()
	case *QwenRealtimeResponseAudioTranscriptDeltaEvent:
		slog.DebugContext(q.ctx, "qwen-realtime received response audio transcript delta",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("delta_len", len(event.Delta)),
		)
		q.listener.OnTranscriptDelta(event.Delta)
	case *QwenRealtimeResponseAudioTranscriptDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response audio transcript done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.Int("transcript_len", len(event.Transcript)),
		)
		q.listener.OnTranscriptDone(event.Transcript)
	case *QwenRealtimeResponseFunctionCallArgumentsDeltaEvent:
		slog.DebugContext(q.ctx, "qwen-realtime received response function call delta",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.String("call_id", event.CallID),
			slog.Int("delta_len", len(event.Delta)),
		)
	case *QwenRealtimeResponseFunctionCallArgumentsDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response function call done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.String("call_id", event.CallID),
			slog.String("name", event.Name),
			slog.Int("arguments_len", len(event.Arguments)),
		)
		q.state.Store(stateToolCalling)
		q.listener.OnToolCall(ToolCall{
			CallID:    event.CallID,
			Name:      event.Name,
			Arguments: event.Arguments,
			Metadata: map[string]string{
				"itemId":     event.ItemID,
				"responseId": event.ResponseID,
			},
		})
	case *QwenRealtimeResponseOutputItemAddedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response output item added",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.Int("output_index", event.OutputIndex),
			slog.String("item_id", event.Item.ID),
			slog.String("item_type", event.Item.Type),
			slog.String("status", event.Item.Status),
			slog.String("call_id", event.Item.CallID),
			slog.String("name", event.Item.Name),
		)
	case *QwenRealtimeResponseOutputItemDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response output item done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.Int("output_index", event.OutputIndex),
			slog.String("item_id", event.Item.ID),
			slog.String("item_type", event.Item.Type),
			slog.String("status", event.Item.Status),
			slog.String("call_id", event.Item.CallID),
			slog.String("name", event.Item.Name),
		)
	case *QwenRealtimeResponseContentPartAddedEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response content part added",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.String("part_type", event.Part.Type),
		)
	case *QwenRealtimeResponseContentPartDoneEvent:
		slog.InfoContext(q.ctx, "qwen-realtime received response content part done",
			slog.String("event_id", event.EventID),
			slog.String("response_id", event.ResponseID),
			slog.String("item_id", event.ItemID),
			slog.Int("output_index", event.OutputIndex),
			slog.Int("content_index", event.ContentIndex),
			slog.String("part_type", event.Part.Type),
			slog.Int("text_len", len(event.Part.Text)),
			slog.Int("transcript_len", len(event.Part.Transcript)),
		)
	}
}

//nolint:gocyclo,funlen // Qwen Realtime event decoding is a protocol discriminator over many event types.
func (q *QwenRealtimeProvider) decodeQwenRealtimeServerEvent(message []byte) (QwenRealtimeServerMessage, error) {
	node, err := sonic.Get(message, "type")
	if err != nil {
		return nil, err
	}
	messageType, err := node.String()
	if err != nil {
		return nil, err
	}

	var serverMessage QwenRealtimeServerMessage
	switch messageType {
	case QwenRealtimeEventError:
		serverMessage = &QwenRealtimeErrorEvent{}
	case QwenRealtimeEventSessionCreated:
		serverMessage = &QwenRealtimeSessionCreatedEvent{}
	case QwenRealtimeEventSessionUpdated:
		serverMessage = &QwenRealtimeSessionUpdatedEvent{}
	case QwenRealtimeEventInputAudioBufferSpeechStarted:
		serverMessage = &QwenRealtimeInputAudioBufferSpeechStartedEvent{}
	case QwenRealtimeEventInputAudioBufferSpeechStopped:
		serverMessage = &QwenRealtimeInputAudioBufferSpeechStoppedEvent{}
	case QwenRealtimeEventInputAudioBufferCommitted:
		serverMessage = &QwenRealtimeInputAudioBufferCommittedEvent{}
	case QwenRealtimeEventInputAudioBufferCleared:
		serverMessage = &QwenRealtimeInputAudioBufferClearedEvent{}
	case QwenRealtimeEventConversationItemCreated:
		serverMessage = &QwenRealtimeConversationItemCreatedEvent{}
	case QwenRealtimeEventInputAudioTranscriptionDelta:
		serverMessage = &QwenRealtimeInputAudioTranscriptionDeltaEvent{}
	case QwenRealtimeEventInputAudioTranscriptionCompleted:
		serverMessage = &QwenRealtimeInputAudioTranscriptionCompletedEvent{}
	case QwenRealtimeEventInputAudioTranscriptionFailed:
		serverMessage = &QwenRealtimeInputAudioTranscriptionFailedEvent{}
	case QwenRealtimeEventResponseCreated:
		serverMessage = &QwenRealtimeResponseCreatedEvent{}
	case QwenRealtimeEventResponseDone:
		serverMessage = &QwenRealtimeResponseDoneEvent{}
	case QwenRealtimeEventResponseTextDelta:
		serverMessage = &QwenRealtimeResponseTextDeltaEvent{}
	case QwenRealtimeEventResponseTextDone:
		serverMessage = &QwenRealtimeResponseTextDoneEvent{}
	case QwenRealtimeEventResponseAudioDelta:
		serverMessage = &QwenRealtimeResponseAudioDeltaEvent{}
	case QwenRealtimeEventResponseAudioDone:
		serverMessage = &QwenRealtimeResponseAudioDoneEvent{}
	case QwenRealtimeEventResponseAudioTranscriptDelta:
		serverMessage = &QwenRealtimeResponseAudioTranscriptDeltaEvent{}
	case QwenRealtimeEventResponseAudioTranscriptDone:
		serverMessage = &QwenRealtimeResponseAudioTranscriptDoneEvent{}
	case QwenRealtimeEventResponseFunctionCallArgumentsDelta:
		serverMessage = &QwenRealtimeResponseFunctionCallArgumentsDeltaEvent{}
	case QwenRealtimeEventResponseFunctionCallArgumentsDone:
		serverMessage = &QwenRealtimeResponseFunctionCallArgumentsDoneEvent{}
	case QwenRealtimeEventResponseOutputItemAdded:
		serverMessage = &QwenRealtimeResponseOutputItemAddedEvent{}
	case QwenRealtimeEventResponseOutputItemDone:
		serverMessage = &QwenRealtimeResponseOutputItemDoneEvent{}
	case QwenRealtimeEventResponseContentPartAdded:
		serverMessage = &QwenRealtimeResponseContentPartAddedEvent{}
	case QwenRealtimeEventResponseContentPartDone:
		serverMessage = &QwenRealtimeResponseContentPartDoneEvent{}
	default:
		return nil, fmt.Errorf("qwen realtime: unsupported server event type %q", messageType)
	}
	if err = sonic.Unmarshal(message, serverMessage); err != nil {
		return nil, err
	}
	return serverMessage, nil
}

func qwenRealtimeTotalTokens(usage *QwenRealtimeUsage) int {
	if usage == nil {
		return 0
	}
	return usage.TotalTokens
}

func qwenRealtimeInputTokens(usage *QwenRealtimeUsage) int {
	if usage == nil {
		return 0
	}
	return usage.InputTokens
}

func qwenRealtimeOutputTokens(usage *QwenRealtimeUsage) int {
	if usage == nil {
		return 0
	}
	return usage.OutputTokens
}
