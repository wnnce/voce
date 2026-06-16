package engine

import (
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPluginBuilder struct {
	name string
}

func (b testPluginBuilder) Name() string                   { return b.name }
func (b testPluginBuilder) Description() string            { return "" }
func (b testPluginBuilder) Schema() *jsonschema.Schema     { return nil }
func (b testPluginBuilder) Inputs() []Property             { return nil }
func (b testPluginBuilder) Outputs() []Property            { return nil }
func (b testPluginBuilder) Ports() []PortMetadata          { return nil }
func (b testPluginBuilder) Build(_ []byte) (Plugin, error) { return &BuiltinPlugin{}, nil }

func TestPluginStore_LoadBuilder(t *testing.T) {
	local := NewPluginResource(LocalNamespace)
	require.NoError(t, local.RegisterBuilder(testPluginBuilder{name: "echo"}))

	remote := NewPluginResource("python")
	require.NoError(t, remote.RegisterBuilder(testPluginBuilder{name: "translate"}))

	store := NewPluginStore(local, remote)

	builder, err := store.LoadBuilder("", "echo")
	require.NoError(t, err)
	assert.Equal(t, "echo", builder.Name())

	builder, err = store.LoadBuilder("python", "translate")
	require.NoError(t, err)
	assert.Equal(t, "translate", builder.Name())

	_, err = store.LoadBuilder("python", "echo")
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

func TestPluginStore_LoadBuilderAmbiguous(t *testing.T) {
	local := NewPluginResource(LocalNamespace)
	require.NoError(t, local.RegisterBuilder(testPluginBuilder{name: "echo"}))

	remote := NewPluginResource("python")
	require.NoError(t, remote.RegisterBuilder(testPluginBuilder{name: "echo"}))

	store := NewPluginStore(local, remote)

	_, err := store.LoadBuilder("", "echo")
	require.ErrorIs(t, err, ErrPluginAmbiguous)

	builder, err := store.LoadBuilder(LocalNamespace, "echo")
	require.NoError(t, err)
	assert.Equal(t, "echo", builder.Name())
}

func TestPluginStore_Resources(t *testing.T) {
	store := NewPluginStore()
	assert.Empty(t, store.ListPluginResources())

	local := NewPluginResource(LocalNamespace)
	store = NewPluginStore(local)

	require.NoError(t, store.AddResource(NewPluginResource("python")))

	err := store.AddResource(NewPluginResource("python"))
	require.ErrorIs(t, err, ErrPluginDuplicate)

	resources := store.ListPluginResources()
	require.Len(t, resources, 2)
	assert.Equal(t, LocalNamespace, resources[0].Namespace())
	assert.Equal(t, "python", resources[1].Namespace())
}

func TestPluginResource_RegisterBuilderErrors(t *testing.T) {
	resource := NewPluginResource("test")

	err := resource.RegisterBuilder(nil)
	require.ErrorIs(t, err, ErrPluginInvalid)

	err = resource.RegisterBuilder(testPluginBuilder{})
	require.ErrorIs(t, err, ErrPluginInvalid)

	require.NoError(t, resource.RegisterBuilder(testPluginBuilder{name: "echo"}))
	err = resource.RegisterBuilder(testPluginBuilder{name: "echo"})
	require.ErrorIs(t, err, ErrPluginDuplicate)
}

func TestPluginStore_ListBuildersStable(t *testing.T) {
	local := NewPluginResource(LocalNamespace)
	require.NoError(t, local.RegisterBuilder(testPluginBuilder{name: "z_local"}))
	require.NoError(t, local.RegisterBuilder(testPluginBuilder{name: "a_local"}))

	remoteB := NewPluginResource("remote_b")
	require.NoError(t, remoteB.RegisterBuilder(testPluginBuilder{name: "b"}))

	remoteA := NewPluginResource("remote_a")
	require.NoError(t, remoteA.RegisterBuilder(testPluginBuilder{name: "c"}))
	require.NoError(t, remoteA.RegisterBuilder(testPluginBuilder{name: "a"}))

	store := NewPluginStore(local, remoteB, remoteA)

	descriptors := store.ListPluginBuilders()
	require.Len(t, descriptors, 5)

	assert.Equal(t, []string{
		"local:a_local",
		"local:z_local",
		"remote_a:a",
		"remote_a:c",
		"remote_b:b",
	}, descriptorNames(descriptors))
}

func descriptorNames(descriptors []PluginDescriptor) []string {
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		names = append(names, descriptor.Namespace+":"+descriptor.Builder.Name())
	}
	return names
}
