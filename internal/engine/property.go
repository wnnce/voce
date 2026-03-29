package engine

import (
	"fmt"
)

const (
	// Standard prefixes defining the domain of a Property contract
	PrefixSignal  = "signal"
	PrefixPayload = "data"
	PrefixAudio   = "audio"
)

type PropertyType string

const (
	TypeString  PropertyType = "string"
	TypeNumber  PropertyType = "number"
	TypeInteger PropertyType = "integer"
	TypeBoolean PropertyType = "boolean"
	TypeArray   PropertyType = "array"
	TypeObject  PropertyType = "object"
	TypeNull    PropertyType = "null"
	TypeAny     PropertyType = ""
)

// Field describes a single key-value pair carried by a Property.
// For event-type Properties, Fields is empty.
type Field struct {
	Key      string       `json:"key"`
	Type     PropertyType `json:"type"`
	Required bool         `json:"required"`
}

// Property represents one logical channel (prefix + name) and the fields it carries.
// An event Property has no Fields; a signal/data/audio/video Property has one or more Fields.
type Property struct {
	Prefix string  `json:"prefix"`
	Name   string  `json:"name"`
	Fields []Field `json:"fields,omitempty"`
}

func (p Property) String() string {
	return fmt.Sprintf("%s:%s", p.Prefix, p.Name)
}

// ValidateProperty checks whether an upstream Property satisfies the contract
// declared by a downstream Property.
//
// Rules:
//  1. Prefix must match.
//  2. If downstream.Name is non-empty, upstream.Name must equal it.
//     If downstream.Name is empty, the name check is skipped (wildcard).
//  3. If downstream has no Fields (signal-only), upstream presence is enough.
//  4. For every required field in downstream, upstream must provide a field
//     with the same key, a compatible type (TypeAny accepts any), and at
//     least the same required strength.
func ValidateProperty(upstream, downstream Property) error {
	if upstream.Prefix != downstream.Prefix {
		return fmt.Errorf("prefix mismatch: upstream %s, downstream %s", upstream.Prefix, downstream.Prefix)
	}
	// Empty downstream Name = wildcard: any upstream name under the same prefix is accepted.
	if downstream.Name != "" && upstream.Name != downstream.Name {
		return fmt.Errorf("name mismatch: upstream %s, downstream %s", upstream.Name, downstream.Name)
	}

	// Signal-only downstream: presence of upstream is enough.
	if len(downstream.Fields) == 0 {
		return nil
	}

	// Index upstream fields by key for fast lookup.
	upFields := make(map[string]Field, len(upstream.Fields))
	for _, f := range upstream.Fields {
		upFields[f.Key] = f
	}

	for _, df := range downstream.Fields {
		if !df.Required {
			continue
		}
		uf, exists := upFields[df.Key]
		if !exists {
			return fmt.Errorf("required field [%s:%s.%s] not provided by upstream", downstream.Prefix, downstream.Name, df.Key)
		}
		if df.Type != TypeAny && uf.Type != df.Type {
			return fmt.Errorf("type mismatch for field [%s:%s.%s]: upstream %s, downstream %s",
				downstream.Prefix, downstream.Name, df.Key, uf.Type, df.Type)
		}
		if !uf.Required {
			return fmt.Errorf("field [%s:%s.%s] is required by downstream but only optionally produced by upstream",
				downstream.Prefix, downstream.Name, df.Key)
		}
	}
	return nil
}

// ValidateProperties performs a hit-based contract check scoped to a specific prefix.
// This ensures that an edge session (e.g. signal type) only validates properties of its own domain.
//
// Rules:
//  1. Filters both upstreams and downstreams by the provided prefix.
//  2. If the downstream node declares no properties for this prefix, validation fails.
//  3. If the upstream node produces no properties for this prefix, validation fails.
//  4. At least one upstream Property of this prefix must match a downstream Property by name.
//  5. For every upstream Property that DOES match a downstream Property, the field contract must match.
//  6. Unrecognized upstream properties within the same prefix domain are ignored.
//
// (Allows for flexible decoupling between data-rich producers and generic consumers).
func ValidateProperties(upstreams, downstreams []Property, prefix string) error {
	var filteredUps []Property
	for _, p := range upstreams {
		if p.Prefix == prefix {
			filteredUps = append(filteredUps, p)
		}
	}

	var filteredDowns []Property
	for _, p := range downstreams {
		if p.Prefix == prefix {
			filteredDowns = append(filteredDowns, p)
		}
	}

	if len(filteredDowns) == 0 {
		return nil
	}
	if len(filteredUps) == 0 {
		return fmt.Errorf("source node does not produce [%s] type signals", prefix)
	}

	downsByName := make(map[string]Property)
	var wildcards []Property
	for _, d := range filteredDowns {
		if d.Name != "" {
			downsByName[d.String()] = d
		} else {
			wildcards = append(wildcards, d)
		}
	}

	hasAnyHit := false
	for _, up := range filteredUps {
		var matchedDown *Property

		// Try exact match first (prefix:name)
		if d, ok := downsByName[up.String()]; ok {
			matchedDown = &d
		} else {
			// Try wildcard match (prefix:*)
			for _, d := range wildcards {
				if d.Name == "" {
					matchedDown = &d
					break
				}
			}
		}

		// If this upstream is not declared/recognized by the downstream, ignore it.
		if matchedDown == nil {
			continue
		}

		// Hit! Mark as recognized and enforce strict field validation.
		hasAnyHit = true
		if err := ValidateProperty(up, *matchedDown); err != nil {
			return fmt.Errorf("contract violation for recognized property [%s]: %w", up.String(), err)
		}
	}

	// 4. Final check: at least one recognized contract must have matched
	if !hasAnyHit {
		return fmt.Errorf("none of the source [%s] properties are recognized by the target plugin", prefix)
	}

	return nil
}

// PropertyBuilder accumulates Property declarations in a fluent API.
// Each Add* call creates or updates the Property entry for that prefix+name.
type PropertyBuilder struct {
	// Use a slice to preserve declaration order; use index map for fast lookup.
	properties []Property
	index      map[string]int // identityKey -> position in properties
}

func NewPropertyBuilder() *PropertyBuilder {
	return &PropertyBuilder{
		index: make(map[string]int),
	}
}

// AddSignalEvent registers a signal-only signal Property (no fields).
// Use this when the downstream only needs to know a signal was sent,
// without carrying any data — the upstream's matching signal:name is
// sufficient to satisfy the contract.
func (b *PropertyBuilder) AddSignalEvent(name string) *PropertyBuilder {
	b.ensureProperty(PrefixSignal, name)
	return b
}

// AddSignal adds one or more fields to a signal Property.
func (b *PropertyBuilder) AddSignal(name, key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixSignal, name, key, tp, required)
	return b
}

// AddPayload adds a field to a data Property.
func (b *PropertyBuilder) AddPayload(name, key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixPayload, name, key, tp, required)
	return b
}

// AddAudio adds a field to an audio Property.
func (b *PropertyBuilder) AddAudio(name, key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixAudio, name, key, tp, required)
	return b
}

// AddWildSignal adds a field to a wildcard signal Property (Name="").
// During validation, this matches any upstream signal Property regardless of name,
// as long as the required field can be found in it.
func (b *PropertyBuilder) AddWildSignal(key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixSignal, "", key, tp, required)
	return b
}

// AddWildPayload adds a field to a wildcard data Property (Name="").
func (b *PropertyBuilder) AddWildPayload(key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixPayload, "", key, tp, required)
	return b
}

// AddWildAudio adds a field to a wildcard audio Property (Name="").
func (b *PropertyBuilder) AddWildAudio(key string, tp PropertyType, required bool) *PropertyBuilder {
	b.addField(PrefixAudio, "", key, tp, required)
	return b
}

// Build returns the accumulated list of Properties.
// The caller should treat the returned slice as read-only.
func (b *PropertyBuilder) Build() []Property {
	return b.properties
}

// ensureProperty returns the index of the Property for the given prefix+name,
// creating it if it does not yet exist.
func (b *PropertyBuilder) ensureProperty(prefix, name string) int {
	k := prefix + ":" + name
	if idx, ok := b.index[k]; ok {
		return idx
	}
	idx := len(b.properties)
	b.properties = append(b.properties, Property{Prefix: prefix, Name: name})
	b.index[k] = idx
	return idx
}

func (b *PropertyBuilder) addField(prefix, name, key string, tp PropertyType, required bool) {
	idx := b.ensureProperty(prefix, name)
	b.properties[idx].Fields = append(b.properties[idx].Fields, Field{
		Key:      key,
		Type:     tp,
		Required: required,
	})
}
