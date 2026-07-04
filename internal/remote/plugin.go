package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
	"google.golang.org/grpc/metadata"
)

const (
	defaultCallLease         = 30 * time.Second
	defaultCancelSendTimeout = time.Second
)

type PluginState int32

const (
	PluginStateCreated PluginState = iota
	PluginStateStarting
	PluginStateStreaming
	PluginStateFailed
	PluginStateStopping
	PluginStateStopped
	PluginStateDestroyed
)

type Plugin struct {
	engine.BuiltinPlugin
	client       pluginv1.RemotePluginServiceClient
	stream       pluginv1.RemotePluginService_RunInstanceClient
	metadata     engine.PluginMetadata
	instanceID   string
	multi        bool
	flow         engine.Flow
	state        atomic.Int32
	createdAt    time.Time
	lastActiveMS atomic.Int64
	writeCh      chan *pluginv1.RuntimeMessage
	cancel       context.CancelFunc
	calls        [2]atomic.Pointer[call]
}

func NewPlugin(
	client pluginv1.RemotePluginServiceClient,
	instanceID string,
	metadata engine.PluginMetadata,
	multi ...bool,
) *Plugin {
	now := time.Now()
	p := &Plugin{
		client:     client,
		instanceID: instanceID,
		metadata:   metadata,
		createdAt:  now,
		writeCh:    make(chan *pluginv1.RuntimeMessage, 128),
	}
	if len(multi) > 0 {
		p.multi = multi[0]
	}
	p.state.Store(int32(PluginStateCreated))
	p.lastActiveMS.Store(now.UnixMilli())
	return p
}

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	if !p.state.CompareAndSwap(int32(PluginStateCreated), int32(PluginStateStarting)) {
		return fmt.Errorf("remote plugin cannot start from state %d: %s", p.State(), p.metadata.Name)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	streamCtx = metadata.NewOutgoingContext(streamCtx, metadata.Pairs("instance-id", p.instanceID))
	stream, err := p.client.RunInstance(streamCtx)
	if err != nil {
		cancel()
		p.setState(PluginStateFailed)
		return err
	}

	p.cancel = cancel
	p.stream = stream
	p.flow = flow
	p.setState(PluginStateStreaming)
	slog.InfoContext(ctx, "remote plugin stream started",
		"plugin", p.metadata.Name,
		"multi", p.multi)

	go p.readLoop()
	go p.writeLoop()

	if err = p.doCall(ctx, &pluginv1.RuntimeMessage{
		Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE,
		Body: &pluginv1.RuntimeMessage_Lifecycle{Lifecycle: &pluginv1.LifecycleEvent{
			Type: pluginv1.LifecycleType_LIFECYCLE_TYPE_START,
		}},
	}); err != nil {
		cancel()
		p.setState(PluginStateFailed)
		return err
	}
	return nil
}

func (p *Plugin) OnReady(ctx context.Context, flow engine.Flow) {
	if err := p.doCall(ctx, &pluginv1.RuntimeMessage{
		Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE,
		Body: &pluginv1.RuntimeMessage_Lifecycle{Lifecycle: &pluginv1.LifecycleEvent{
			Type: pluginv1.LifecycleType_LIFECYCLE_TYPE_READY,
		}},
	}); err != nil {
		slog.ErrorContext(ctx, "remote plugin OnReady failed",
			"plugin", p.metadata.Name, "error", err)
	}
}

func (p *Plugin) OnPause(ctx context.Context) {
	if err := p.doCall(ctx, &pluginv1.RuntimeMessage{
		Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE,
		Body: &pluginv1.RuntimeMessage_Lifecycle{Lifecycle: &pluginv1.LifecycleEvent{
			Type: pluginv1.LifecycleType_LIFECYCLE_TYPE_PAUSE,
		}},
	}); err != nil {
		slog.ErrorContext(ctx, "remote plugin OnPause failed",
			"plugin", p.metadata.Name, "error", err)
	}
}

func (p *Plugin) OnResume(ctx context.Context, flow engine.Flow) {
	if err := p.doCall(ctx, &pluginv1.RuntimeMessage{
		Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE,
		Body: &pluginv1.RuntimeMessage_Lifecycle{Lifecycle: &pluginv1.LifecycleEvent{
			Type: pluginv1.LifecycleType_LIFECYCLE_TYPE_RESUME,
		}},
	}); err != nil {
		slog.ErrorContext(ctx, "remote plugin OnResume failed",
			"plugin", p.metadata.Name, "error", err)
	}
}

func (p *Plugin) OnStop() {
	if p.State() >= PluginStateStopping {
		return
	}

	// Send STOP lifecycle while still in Streaming so doCall won't reject.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.doCall(ctx, &pluginv1.RuntimeMessage{
		Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE,
		Body: &pluginv1.RuntimeMessage_Lifecycle{Lifecycle: &pluginv1.LifecycleEvent{
			Type: pluginv1.LifecycleType_LIFECYCLE_TYPE_STOP,
		}},
	})

	// Stream closed, but instance still exists on remote side.
	if p.cancel != nil {
		p.cancel()
	}
	p.setState(PluginStateStopped)
	slog.Info("remote plugin stopped",
		"plugin", p.metadata.Name)
}

func (p *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	p.callRemoteSchemaEvent(ctx, "signal", signal.Name(), signal, func(props []byte) *pluginv1.RuntimeMessage {
		return &pluginv1.RuntimeMessage{
			Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL,
			Body: &pluginv1.RuntimeMessage_Signal{Signal: &pluginv1.SignalEvent{
				Name:       signal.Name(),
				Properties: props,
			}},
		}
	})
}

func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	p.callRemoteSchemaEvent(ctx, "payload", payload.Name(), payload, func(props []byte) *pluginv1.RuntimeMessage {
		return &pluginv1.RuntimeMessage{
			Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD,
			Body: &pluginv1.RuntimeMessage_Payload{Payload: &pluginv1.PayloadEvent{
				Name:       payload.Name(),
				Properties: props,
			}},
		}
	})
}

func (p *Plugin) callRemoteSchemaEvent(
	ctx context.Context,
	eventKind string,
	eventName string,
	value any,
	makeMessage func([]byte) *pluginv1.RuntimeMessage,
) {
	props, err := sonic.Marshal(value)
	if err != nil {
		slog.ErrorContext(ctx, "remote plugin serialize event failed",
			"plugin", p.metadata.Name, "event_type", eventKind, "event", eventName, "error", err)
		return
	}
	message := makeMessage(props)
	if err = p.doCall(ctx, message, p.withDefaultCancelCallback(message)); err != nil {
		slog.ErrorContext(ctx, "remote plugin event failed",
			"plugin", p.metadata.Name, "event_type", eventKind, "event", eventName, "error", err)
	}
}

// OnAudio and OnVideo are inherited from engine.BuiltinPlugin (passthrough).

func (p *Plugin) doCall(
	ctx context.Context,
	message *pluginv1.RuntimeMessage,
	options ...callOption,
) error {
	if p.State() != PluginStateStreaming {
		return fmt.Errorf("remote plugin is not streaming: %s state=%d", p.metadata.Name, p.State())
	}
	if message.MessageId == "" {
		message.MessageId = uuid.New().String()
	}
	message.InstanceId = p.instanceID
	current := newCall(ctx, message.MessageId, defaultCallLease, options...)
	idx := p.index(message.Type)
	if !p.calls[idx].CompareAndSwap(nil, current) {
		slog.WarnContext(ctx, "remote call slot busy",
			"plugin", p.metadata.Name,
			"slot", idx)
		return fmt.Errorf("remote call already in progress: %s", p.metadata.Name)
	}
	defer p.calls[idx].CompareAndSwap(current, nil)

	if err := syncx.SendWithContext(ctx, p.writeCh, message); err != nil {
		current.finish(err)
		if !errors.Is(err, context.Canceled) {
			p.setState(PluginStateFailed)
		}
		return fmt.Errorf("send remote runtime message %s: %w", message.MessageId, err)
	}
	if err := current.wait(); err != nil {
		// Context cancellation (e.g. interruption) is a normal flow, not a fatal error
		if !errors.Is(err, context.Canceled) {
			p.setState(PluginStateFailed)
		}
		return err
	}
	return nil
}

func (p *Plugin) withDefaultCancelCallback(message *pluginv1.RuntimeMessage) callOption {
	return withCancelCallback(func() bool {
		cancelMsg := &pluginv1.RuntimeMessage{
			InstanceId:    p.instanceID,
			MessageId:     uuid.New().String(),
			CorrelationId: message.GetMessageId(),
			Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_CANCEL,
			Body:          &pluginv1.RuntimeMessage_Cancel{Cancel: &pluginv1.CancelEvent{}},
		}
		ctx := context.Background()
		if p.stream != nil {
			ctx = p.stream.Context()
		}
		if err := syncx.SendWithTimeout(ctx, p.writeCh, cancelMsg, defaultCancelSendTimeout); err != nil {
			slog.WarnContext(ctx, "remote plugin send cancel failed",
				"plugin", p.metadata.Name,
				"error", err)
			return true
		}
		slog.InfoContext(ctx, "remote plugin cancel sent",
			"plugin", p.metadata.Name)
		return false
	})
}

func (p *Plugin) index(messageType pluginv1.RuntimeMessageType) int {
	if !p.multi {
		return 0
	}
	switch messageType {
	case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL:
		return 0
	case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD:
		return 1
	default:
		return 0
	}
}

func (p *Plugin) writeLoop() {
	stream := p.stream
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "remote plugin write loop stopped",
				"plugin", p.metadata.Name)
			return
		case msg, ok := <-p.writeCh:
			if !ok {
				return
			}
			if err := stream.Send(msg); err != nil {
				slog.ErrorContext(ctx, "remote plugin stream send failed",
					"plugin", p.metadata.Name,
					"error", err,
				)
				for i := range len(p.calls) {
					current := p.calls[i].Load()
					messageId := msg.MessageId
					if msg.Type == pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_CANCEL {
						messageId = msg.CorrelationId
					}
					if current == nil || current.id != messageId {
						continue
					}
					current.finish(err)
				}
			}
		}
	}
}

func (p *Plugin) readLoop() {
	stream := p.stream
	p.Touch()
	ctx := stream.Context()
	defer func() {
		p.finishCalls(fmt.Errorf("remote plugin stream closed: %s", p.metadata.Name))
		if p.cancel != nil {
			p.cancel()
		}
		slog.InfoContext(ctx, "remote plugin read loop stopped",
			"plugin", p.metadata.Name,
			"state", p.State())
	}()
	for {
		message, err := stream.Recv()
		if err != nil {
			if (ctx.Err() != nil || errors.Is(err, io.EOF)) && p.State() != PluginStateFailed {
				p.setState(PluginStateStopped)
				return
			}
			p.setState(PluginStateFailed)
			slog.ErrorContext(ctx, "remote plugin stream recv failed",
				"plugin", p.metadata.Name,
				"error", err)
			return
		}
		p.Touch()
		switch message.GetType() {
		case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_ACK:
			p.handleAck(ctx, message)
		case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT:
			p.handleReport(ctx, message)
		case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL:
			p.handleEmitSignal(ctx, message)
		case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD:
			p.handleEmitPayload(ctx, message)
		case pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_EMIT_LOG:
			p.handleEmitLog(ctx, message)
		default:
			slog.WarnContext(ctx, "remote plugin stream received unsupported message",
				"plugin", p.metadata.Name,
				"type", message.GetType())
		}
	}
}

func (p *Plugin) handleAck(_ context.Context, message *pluginv1.RuntimeMessage) {
	for i := range len(p.calls) {
		current := p.calls[i].Load()
		if current == nil || current.id != message.GetCorrelationId() {
			continue
		}
		current.renew()
		return
	}
}

func (p *Plugin) handleReport(ctx context.Context, message *pluginv1.RuntimeMessage) {
	for i := range len(p.calls) {
		current := p.calls[i].Load()
		if current == nil || current.id != message.GetCorrelationId() {
			continue
		}
		slog.DebugContext(ctx, "remote call report received",
			"plugin", p.metadata.Name,
			"status", message.GetReport().GetStatus(),
			"slot", i)
		current.finish(reportError(message.GetReport()))
		return
	}
	slog.WarnContext(ctx, "remote call report has no matching call",
		"plugin", p.metadata.Name,
		"status", message.GetReport().GetStatus())
}

func (p *Plugin) finishCalls(err error) {
	for i := range len(p.calls) {
		current := p.calls[i].Load()
		if current != nil {
			current.finish(err)
		}
	}
}

func (p *Plugin) handleEmitSignal(ctx context.Context, message *pluginv1.RuntimeMessage) {
	emit := message.GetEmitSignal()
	event := emit.GetSignal()
	if event == nil {
		return
	}
	signal := schema.NewSignal(event.GetName())
	if !p.decodeRemoteProperties(ctx, "signal", event.GetName(), event.GetProperties(), signal) {
		return
	}
	sendWithPort(emit.GetPort(), signal.ReadOnly(), p.flow.SendSignal, p.flow.SendSignalToPort)
}

func (p *Plugin) handleEmitPayload(ctx context.Context, message *pluginv1.RuntimeMessage) {
	emit := message.GetEmitPayload()
	event := emit.GetPayload()
	if event == nil {
		return
	}
	payload := schema.NewPayload(event.GetName())
	if !p.decodeRemoteProperties(ctx, "payload", event.GetName(), event.GetProperties(), payload) {
		return
	}
	sendWithPort(emit.GetPort(), payload.ReadOnly(), p.flow.SendPayload, p.flow.SendPayloadToPort)
}

func (p *Plugin) decodeRemoteProperties(
	ctx context.Context,
	eventKind string,
	eventName string,
	data []byte,
	target schema.Properties,
) bool {
	if len(data) == 0 {
		return true
	}
	if err := sonic.Unmarshal(data, target); err != nil {
		slog.ErrorContext(ctx, "remote plugin emitted invalid event properties",
			"plugin", p.metadata.Name,
			"event_type", eventKind,
			"event", eventName,
			"error", err)
		return false
	}
	return true
}

func (p *Plugin) handleEmitLog(ctx context.Context, message *pluginv1.RuntimeMessage) {
	emit := message.GetEmitLog()
	args := []any{
		"plugin", p.metadata.Name,
		"instance_id", p.instanceID,
	}
	if mid := message.GetMessageId(); mid != "" {
		args = append(args, "message_id", mid)
	}
	if cid := message.GetCorrelationId(); cid != "" {
		args = append(args, "correlation_id", cid)
	}
	for key, value := range emit.GetFields() {
		args = append(args, key, value)
	}
	switch emit.GetLevel() {
	case pluginv1.LogLevel_LOG_LEVEL_DEBUG:
		slog.DebugContext(ctx, emit.GetMessage(), args...)
	case pluginv1.LogLevel_LOG_LEVEL_WARN:
		slog.WarnContext(ctx, emit.GetMessage(), args...)
	case pluginv1.LogLevel_LOG_LEVEL_ERROR:
		slog.ErrorContext(ctx, emit.GetMessage(), args...)
	default:
		slog.InfoContext(ctx, emit.GetMessage(), args...)
	}
}

// sendWithPort dispatches a value to a specific port or broadcasts.
func sendWithPort[T any](port int32, value T, send func(T), sendToPort func(int, T)) {
	if port > 0 && int(port) < engine.MaxPortCount {
		sendToPort(int(port), value)
		return
	}
	send(value)
}

func reportError(report *pluginv1.EventReport) error {
	switch report.GetStatus() {
	case pluginv1.ReportStatus_REPORT_STATUS_OK:
		return nil
	case pluginv1.ReportStatus_REPORT_STATUS_CANCELED:
		return nil
	case pluginv1.ReportStatus_REPORT_STATUS_ERROR:
		err := report.GetError()
		return fmt.Errorf("remote call failed: code=%s message=%s details=%s",
			err.GetCode(),
			err.GetMessage(),
			err.GetDetails())
	default:
		return fmt.Errorf("remote call report status is invalid: %s", report.GetStatus())
	}
}

func (p *Plugin) InstanceID() string {
	return p.instanceID
}

func (p *Plugin) Name() string {
	return p.metadata.Name
}

func (p *Plugin) State() PluginState {
	return PluginState(p.state.Load())
}

func (p *Plugin) CreatedAt() time.Time {
	return p.createdAt
}

func (p *Plugin) LastActive() time.Time {
	return time.UnixMilli(p.lastActiveMS.Load())
}

func (p *Plugin) Touch() {
	p.lastActiveMS.Store(time.Now().UnixMilli())
}

func (p *Plugin) setState(state PluginState) {
	p.state.Store(int32(state))
	p.Touch()
}
