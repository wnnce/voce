package engine

import (
	"reflect"

	"github.com/invopop/jsonschema"
)

type PluginFactory[T PluginConfig] func(configure T) Plugin

type (
	// PluginConfig defines the contract for a plugin's configuration,
	// including its JSON schema and decoding logic.
	PluginConfig interface {
		Schema() *jsonschema.Schema
		Decode(data []byte) error
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
	if typ != nil && typ.Kind() == reflect.Pointer {
		newVal := reflect.New(typ.Elem())
		reflect.ValueOf(&zero).Elem().Set(newVal)
	}
	return zero.Schema()
}

func (b *GenericBuilder[T]) Build(data []byte) (Plugin, error) {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ != nil && typ.Kind() == reflect.Pointer {
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
