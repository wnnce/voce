package webrtc

import (
	"fmt"
	"sync"
)

// Registry manages multiple registered WebRTC providers and tracks the default provider.
type Registry struct {
	mu          sync.RWMutex
	providers   map[string]Provider
	order       []string
	defaultName string
}

// NewRegistry initializes an empty WebRTC provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register registers a provider. The first registered provider
// automatically becomes the default if no default has been set yet.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("%w: provider is nil", ErrInvalidConfig)
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("%w: provider name is empty", ErrInvalidConfig)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; !exists {
		r.order = append(r.order, name)
	}
	r.providers[name] = p
	if r.defaultName == "" {
		r.defaultName = name
	}
	return nil
}

// SetDefault sets the default provider name.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name != "" {
		if _, ok := r.providers[name]; !ok {
			return fmt.Errorf("%w: unknown provider %q", ErrInvalidConfig, name)
		}
	}
	r.defaultName = name
	return nil
}

// Get retrieves a provider by name. If name is empty, it returns the default provider.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.defaultName
	}
	p, ok := r.providers[name]
	return p, ok
}

// Default returns the default provider name.
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultName
}

// List returns all registered providers in registration order.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Provider, 0, len(r.order))
	for _, name := range r.order {
		if p, ok := r.providers[name]; ok {
			list = append(list, p)
		}
	}
	return list
}

// Len returns the count of registered providers.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}
