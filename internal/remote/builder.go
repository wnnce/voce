package remote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/invopop/jsonschema"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/internal/engine"
)

const prefixVideo = "video"
const defaultPayloadTrackBufferSize = 128

type Builder struct {
	client     *Client
	meta       engine.PluginMetadata
	schema     *jsonschema.Schema
	multiTrack *pluginv1.MultiTrackConfig
}

func NewBuilder(client *Client, metadata *pluginv1.PluginMetadata) (*Builder, error) {
	if metadata.GetName() == "" {
		return nil, fmt.Errorf("remote plugin name is empty")
	}

	schema, err := decodeSchema(metadata.GetSchema())
	if err != nil {
		return nil, fmt.Errorf("decode remote plugin schema %s: %w", metadata.GetName(), err)
	}

	return &Builder{
		client:     client,
		meta:       convertPluginMetadata(metadata),
		schema:     schema,
		multiTrack: metadata.GetMultiTrack(),
	}, nil
}

func (b *Builder) Name() string {
	return b.meta.Name
}

func (b *Builder) Description() string {
	return b.meta.Description
}

func (b *Builder) Schema() *jsonschema.Schema {
	return b.schema
}

func (b *Builder) Inputs() []engine.Property {
	return b.meta.Inputs
}

func (b *Builder) Outputs() []engine.Property {
	return b.meta.Outputs
}

func (b *Builder) Ports() []engine.PortMetadata {
	return b.meta.Ports
}

func (b *Builder) Build(data []byte) (engine.Plugin, error) {
	if b.client == nil {
		return nil, fmt.Errorf("remote plugin client is nil: %s", b.Name())
	}

	instanceID := uuid.New().String()
	if err := b.client.CreateInstance(context.Background(), instanceID, b.Name(), data); err != nil {
		return nil, err
	}

	options := convertMultiTrackOptions(b.multiTrack)
	plugin := NewPlugin(b.client.RPC(), instanceID, b.meta, len(options) > 0)
	if err := b.client.RegisterInstance(plugin); err != nil {
		_ = b.client.DestroyInstance(context.Background(), instanceID, "register instance failed")
		return nil, err
	}
	if len(options) > 0 {
		return engine.NewMultiTrackPlugin(plugin, options...), nil
	}
	return plugin, nil
}

func convertMultiTrackOptions(config *pluginv1.MultiTrackConfig) []engine.TrackOption {
	if config == nil || !config.GetEnabled() {
		return nil
	}
	payload := config.GetPayload()
	if payload == nil || !payload.GetEnabled() {
		return nil
	}
	bufferSize := int(payload.GetBufferSize())
	if bufferSize <= 0 {
		bufferSize = defaultPayloadTrackBufferSize
	}
	return []engine.TrackOption{
		engine.WithPayloadTrack(
			bufferSize,
			convertDropStrategy(payload.GetDropStrategy()),
			payload.GetInterruptSignals()...,
		),
	}
}

func convertDropStrategy(value pluginv1.DropStrategy) engine.DropStrategy {
	switch value {
	case pluginv1.DropStrategy_DROP_STRATEGY_DROP_NEWEST:
		return engine.DropNewest
	case pluginv1.DropStrategy_DROP_STRATEGY_DROP_OLDEST:
		return engine.DropOldest
	default:
		return engine.BlockIfFull
	}
}

func decodeSchema(data string) (*jsonschema.Schema, error) {
	if data == "" {
		return nil, nil
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(data), &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func convertPluginMetadata(metadata *pluginv1.PluginMetadata) engine.PluginMetadata {
	return engine.PluginMetadata{
		Name:        metadata.GetName(),
		Description: metadata.GetDescription(),
		Inputs:      convertProperties(metadata.GetInputs()),
		Outputs:     convertProperties(metadata.GetOutputs()),
		Ports:       convertPorts(metadata.GetPorts()),
	}
}

func convertProperties(items []*pluginv1.Property) []engine.Property {
	properties := make([]engine.Property, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		properties = append(properties, engine.Property{
			Prefix: convertEventType(item.GetType()),
			Name:   item.GetName(),
			Fields: convertFields(item.GetFields()),
		})
	}
	return properties
}

func convertFields(items []*pluginv1.Field) []engine.Field {
	fields := make([]engine.Field, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		fields = append(fields, engine.Field{
			Key:      item.GetKey(),
			Type:     convertValueType(item.GetType()),
			Required: item.GetRequired(),
		})
	}
	return fields
}

func convertPorts(items []*pluginv1.PortMetadata) []engine.PortMetadata {
	ports := make([]engine.PortMetadata, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ports = append(ports, engine.PortMetadata{
			Type:        convertPortEventType(item.GetType()),
			Port:        int(item.GetPort()),
			Name:        item.GetName(),
			Description: item.GetDescription(),
		})
	}
	return ports
}

func convertPortEventType(value pluginv1.EventType) engine.EventType {
	switch value {
	case pluginv1.EventType_EVENT_TYPE_SIGNAL:
		return engine.EventSignal
	case pluginv1.EventType_EVENT_TYPE_PAYLOAD:
		return engine.EventPayload
	case pluginv1.EventType_EVENT_TYPE_AUDIO:
		return engine.EventAudio
	case pluginv1.EventType_EVENT_TYPE_VIDEO:
		return engine.EventVideo
	default:
		return 0
	}
}

func convertEventType(value pluginv1.EventType) string {
	switch value {
	case pluginv1.EventType_EVENT_TYPE_SIGNAL:
		return engine.PrefixSignal
	case pluginv1.EventType_EVENT_TYPE_PAYLOAD:
		return engine.PrefixPayload
	case pluginv1.EventType_EVENT_TYPE_AUDIO:
		return engine.PrefixAudio
	case pluginv1.EventType_EVENT_TYPE_VIDEO:
		return prefixVideo
	default:
		return ""
	}
}

func convertValueType(value pluginv1.ValueType) engine.PropertyType {
	switch value {
	case pluginv1.ValueType_VALUE_TYPE_STRING:
		return engine.TypeString
	case pluginv1.ValueType_VALUE_TYPE_NUMBER:
		return engine.TypeNumber
	case pluginv1.ValueType_VALUE_TYPE_INTEGER:
		return engine.TypeInteger
	case pluginv1.ValueType_VALUE_TYPE_BOOLEAN:
		return engine.TypeBoolean
	case pluginv1.ValueType_VALUE_TYPE_OBJECT:
		return engine.TypeObject
	case pluginv1.ValueType_VALUE_TYPE_ARRAY:
		return engine.TypeArray
	default:
		return engine.TypeAny
	}
}
