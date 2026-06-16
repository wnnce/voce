package engine

import (
	"fmt"
	"sort"
	"sync"
)

const LocalNamespace = "local"

type PluginResource struct {
	namespace string
	mutex     sync.RWMutex
	builders  map[string]PluginBuilder
}

func NewPluginResource(namespace string) *PluginResource {
	return &PluginResource{
		namespace: namespace,
		builders:  make(map[string]PluginBuilder),
	}
}

func (r *PluginResource) Namespace() string {
	return r.namespace
}

func (r *PluginResource) RegisterBuilder(builder PluginBuilder) error {
	if builder == nil {
		return fmt.Errorf("%w: builder is nil", ErrPluginInvalid)
	}
	name := builder.Name()
	if name == "" {
		return fmt.Errorf("%w: builder name is empty", ErrPluginInvalid)
	}
	for _, port := range builder.Ports() {
		if err := port.Validate(); err != nil {
			return err
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, ok := r.builders[name]; ok {
		return fmt.Errorf("%w: %s in resource %s", ErrPluginDuplicate, name, r.namespace)
	}
	r.builders[name] = builder
	return nil
}

func (r *PluginResource) LoadBuilder(name string) PluginBuilder {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.builders[name]
}

func (r *PluginResource) ListBuilders() []PluginBuilder {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	sort.Strings(names)

	builders := make([]PluginBuilder, 0, len(names))
	for _, name := range names {
		builders = append(builders, r.builders[name])
	}
	return builders
}
