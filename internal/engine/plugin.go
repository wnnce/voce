package engine

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/schema"
)

type PluginFactory[T PluginConfig] func(configure T) Plugin

type (
	// PluginConfig defines the contract for a plugin's configuration,
	// including its JSON schema and decoding logic.
	PluginConfig interface {
		Schema() *jsonschema.Schema
		Decode(data []byte) error
	}

	PortMetadata struct {
		Type        EventType `json:"type"`
		Port        int       `json:"port"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
	}

	PluginMetadata struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Inputs      []Property     `json:"inputs"`
		Outputs     []Property     `json:"outputs"`
		Ports       []PortMetadata `json:"ports"`
	}

	// Plugin defines the processing logic for a node.
	// Each method represents a lifecycle stage or a data processing hook.
	Plugin interface {
		OnStart(ctx context.Context, flow Flow) error
		OnReady(ctx context.Context, flow Flow)
		OnPause(ctx context.Context)
		OnResume(ctx context.Context, flow Flow)
		OnStop()
		OnSignal(ctx context.Context, flow Flow, signal schema.Signal)
		OnPayload(ctx context.Context, flow Flow, payload schema.Payload)
		OnAudio(ctx context.Context, flow Flow, audio schema.Audio)
		OnVideo(ctx context.Context, flow Flow, video schema.Video)
	}

	// PluginBuilder is responsible for instantiating plugins with specific configurations.
	PluginBuilder interface {
		Name() string
		Description() string
		Schema() *jsonschema.Schema
		Inputs() []Property
		Outputs() []Property
		Ports() []PortMetadata
		Build(data []byte) (Plugin, error)
	}
)

var (
	plugins = make(map[string]PluginBuilder)
	mutex   sync.RWMutex
)

func RegisterPlugin[T PluginConfig](factory PluginFactory[T], meta PluginMetadata) error {
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := plugins[meta.Name]; ok {
		return fmt.Errorf("plugin %s already registered", meta.Name)
	}

	for _, port := range meta.Ports {
		if err := port.Validate(); err != nil {
			return err
		}
	}

	plugins[meta.Name] = &GenericBuilder[T]{
		meta:    meta,
		factory: factory,
	}
	return nil
}

func LoadPluginBuilder(name string) PluginBuilder {
	mutex.RLock()
	defer mutex.RUnlock()
	return plugins[name]
}

func GetPluginBuilders() []PluginBuilder {
	mutex.RLock()
	defer mutex.RUnlock()
	list := make([]PluginBuilder, 0, len(plugins))
	for _, b := range plugins {
		list = append(list, b)
	}
	return list
}

func (p *PortMetadata) Validate() error {
	if p.Port <= 0 {
		return fmt.Errorf("port index %d must be greater than 0 (0 is reserved for broadcast)", p.Port)
	}
	if p.Port >= MaxPortCount {
		return fmt.Errorf("port index %d exceeds maximum allowed port (%d)", p.Port, MaxPortCount-1)
	}
	return nil
}

type GenericBuilder[T PluginConfig] struct {
	meta    PluginMetadata
	factory PluginFactory[T]
}

func (b *GenericBuilder[T]) Name() string {
	return b.meta.Name
}

func (b *GenericBuilder[T]) Description() string {
	return b.meta.Description
}

func (b *GenericBuilder[T]) Inputs() []Property {
	return b.meta.Inputs
}

func (b *GenericBuilder[T]) Outputs() []Property {
	return b.meta.Outputs
}

func (b *GenericBuilder[T]) Ports() []PortMetadata {
	return b.meta.Ports
}

func (b *GenericBuilder[T]) Schema() *jsonschema.Schema {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ != nil && typ.Kind() == reflect.Ptr {
		newVal := reflect.New(typ.Elem())
		reflect.ValueOf(&zero).Elem().Set(newVal)
	}
	return zero.Schema()
}

func (b *GenericBuilder[T]) Build(data []byte) (Plugin, error) {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ != nil && typ.Kind() == reflect.Ptr {
		newVal := reflect.New(typ.Elem())
		reflect.ValueOf(&zero).Elem().Set(newVal)
	}
	if err := zero.Decode(data); err != nil {
		return nil, err
	}
	return b.factory(zero), nil
}

type EmptyPluginConfig struct {
}

func (e EmptyPluginConfig) Schema() *jsonschema.Schema {
	return nil
}
func (e EmptyPluginConfig) Decode(_ []byte) error {
	return nil
}

type BuiltinPlugin struct{}

func (b *BuiltinPlugin) OnStart(_ context.Context, _ Flow) error {
	return nil
}

func (b *BuiltinPlugin) OnReady(_ context.Context, _ Flow) {}

func (b *BuiltinPlugin) OnPause(_ context.Context) {}

func (b *BuiltinPlugin) OnResume(_ context.Context, _ Flow) {}

func (b *BuiltinPlugin) OnStop() {}

func (b *BuiltinPlugin) OnSignal(_ context.Context, flow Flow, signal schema.Signal) {
	flow.SendSignal(signal)
}

func (b *BuiltinPlugin) OnPayload(_ context.Context, flow Flow, payload schema.Payload) {
	flow.SendPayload(payload)
}

func (b *BuiltinPlugin) OnAudio(_ context.Context, flow Flow, audio schema.Audio) {
	flow.SendAudio(audio)
}

func (b *BuiltinPlugin) OnVideo(_ context.Context, flow Flow, video schema.Video) {
	flow.SendVideo(video)
}
