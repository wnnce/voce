package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrPluginInvalid   = errors.New("plugin invalid")
	ErrPluginDuplicate = errors.New("plugin duplicate")
	ErrPluginNotFound  = errors.New("plugin not found")
	ErrPluginAmbiguous = errors.New("plugin ambiguous")
	ErrPluginForbidden = errors.New("plugin forbidden")
)

type PluginDescriptor struct {
	Namespace string
	Builder   PluginBuilder
}

type PluginStore struct {
	mutex     sync.RWMutex
	resources map[string]*PluginResource
}

func NewPluginStore(resources ...*PluginResource) *PluginStore {
	store := &PluginStore{
		resources: make(map[string]*PluginResource),
	}
	for _, resource := range resources {
		if resource == nil || resource.Namespace() == "" {
			continue
		}
		store.resources[resource.Namespace()] = resource
	}
	return store
}

func (s *PluginStore) AddResource(resource *PluginResource) error {
	if resource == nil {
		return fmt.Errorf("%w: resource is nil", ErrPluginInvalid)
	}
	namespace := resource.Namespace()
	if namespace == "" {
		return fmt.Errorf("%w: resource namespace is empty", ErrPluginInvalid)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.resources[namespace]; ok {
		return fmt.Errorf("%w: resource %s", ErrPluginDuplicate, namespace)
	}
	s.resources[namespace] = resource
	return nil
}

func (s *PluginStore) RemoveResource(namespace string) {
	if strings.TrimSpace(namespace) == "" || namespace == LocalNamespace {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.resources, namespace)
}

func (s *PluginStore) LoadBuilder(namespace, name string) (PluginBuilder, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is empty", ErrPluginInvalid)
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if namespace != "" {
		resource := s.resources[namespace]
		if resource == nil {
			return nil, fmt.Errorf("%w: resource %s", ErrPluginNotFound, namespace)
		}
		builder := resource.LoadBuilder(name)
		if builder == nil {
			return nil, fmt.Errorf("%w: %s in resource %s", ErrPluginNotFound, name, namespace)
		}
		return builder, nil
	}
	var matched PluginDescriptor
	for resourceNamespace, resource := range s.resources {
		builder := resource.LoadBuilder(name)
		if builder == nil {
			continue
		}
		if matched.Builder != nil {
			return nil, fmt.Errorf("%w: %s between resources %s and %s",
				ErrPluginAmbiguous, name, matched.Namespace, resourceNamespace)
		}
		matched = PluginDescriptor{
			Namespace: resourceNamespace,
			Builder:   builder,
		}
	}
	if matched.Builder == nil {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}
	return matched.Builder, nil
}

func (s *PluginStore) ListPluginResources() []*PluginResource {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	namespaces := make([]string, 0, len(s.resources))
	for namespace := range s.resources {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	resources := make([]*PluginResource, 0, len(namespaces))
	for _, namespace := range namespaces {
		resources = append(resources, s.resources[namespace])
	}
	return resources
}

func (s *PluginStore) ListPluginBuilders() []PluginDescriptor {
	resources := s.ListPluginResources()

	descriptors := make([]PluginDescriptor, 0)
	for _, resource := range resources {
		namespace := resource.Namespace()
		for _, builder := range resource.ListBuilders() {
			descriptors = append(descriptors, PluginDescriptor{
				Namespace: namespace,
				Builder:   builder,
			})
		}
	}
	sort.SliceStable(descriptors, func(i, j int) bool {
		if descriptors[i].Namespace != descriptors[j].Namespace {
			return descriptors[i].Namespace < descriptors[j].Namespace
		}
		return descriptors[i].Builder.Name() < descriptors[j].Builder.Name()
	})
	return descriptors
}
