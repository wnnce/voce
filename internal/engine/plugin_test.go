package engine

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ValueConfig implements PluginConfig with value receiver.
// This matches RegisterPlugin[ValueConfig].
type ValueConfig struct {
	AppID string `json:"app_id" jsonschema:"title=App ID"`
}

func (m ValueConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(m)
}

func (m ValueConfig) Decode(data []byte) error {
	// Logic-wise this is useless for the caller if m is a value,
	// but it satisfies the interface for RegisterPlugin[ValueConfig].
	return sonic.Unmarshal(data, &m)
}

// PointerConfig implements PluginConfig with pointer receiver.
// This matches RegisterPlugin[*PointerConfig].
type PointerConfig struct {
	AppID string `json:"app_id" jsonschema:"title=App ID"`
}

func (m *PointerConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(m)
}

func (m *PointerConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, m)
}

type MockPlugin struct {
	BuiltinPlugin
}

func TestPluginRegistry_ConfigTypes(t *testing.T) {
	t.Run("Value Type Registration (Schema Test)", func(t *testing.T) {
		name := "value_ext"
		factory := func(cfg ValueConfig) Plugin {
			return &MockPlugin{}
		}

		// This works because ValueConfig implements PluginConfig (value receiver Decode)
		err := RegisterPlugin(factory, PluginMetadata{Name: name})
		require.NoError(t, err)
		builder := LoadPluginBuilder(name)
		require.NotNil(t, builder)

		// Check Schema correctly identifies the struct
		schema := builder.Schema()
		assert.NotNil(t, schema)
		assert.Contains(t, schema.Definitions, "ValueConfig")
	})

	t.Run("Pointer Type Registration (Full Flow Test)", func(t *testing.T) {
		name := "ptr_ext"
		var capturedAppID string
		factory := func(cfg *PointerConfig) Plugin {
			if cfg != nil {
				capturedAppID = cfg.AppID
			}
			return &MockPlugin{}
		}

		// This works because *PointerConfig implements PluginConfig
		err := RegisterPlugin[*PointerConfig](factory, PluginMetadata{
			Name: name,
		})
		require.NoError(t, err)
		builder := LoadPluginBuilder(name)
		require.NotNil(t, builder)

		// Check Schema correctly identifies the underlying struct even from pointer
		schema := builder.Schema()
		assert.NotNil(t, schema)
		assert.Contains(t, schema.Definitions, "PointerConfig")

		// Build - Verify data is actually injected
		data := []byte(`{"app_id": "ptr_app"}`)
		_, err = builder.Build(data)
		require.NoError(t, err)
		assert.Equal(t, "ptr_app", capturedAppID)
	})
}
