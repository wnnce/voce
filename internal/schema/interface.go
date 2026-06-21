package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/wnnce/voce/pkg/buf"
)

var (
	ErrKeyNotFound        = errors.New("schema: key not found")
	ErrTypeMismatch       = errors.New("schema: type mismatch")
	ErrNotSerializedBytes = errors.New("schema: value is not serialized bytes")
	ErrReadOnly           = errors.New("schema: object is readonly")
)

type entry struct {
	key string
	val any
}

// RefCountable enables manual reference counting for objects that are shared
// across multiple nodes and should be recycled to an object pool.
type RefCountable interface {
	Retain()  // Increment the reference count
	Release() // Decrement the reference count; recycle if zero
}

// ReadOnly provides an immutable view of a property set, serving as a basis
// for zero-allocation data passing.
type ReadOnly interface {
	Get(key string) (any, bool)
	Bind(key string, value any) error // Bind unmarshal the value into a destination object
	Unpack(dst any) error             // Unpack properties into dst (schema → struct)
}

type Properties interface {
	ReadOnly
	Set(key string, value any) error
	Pack(src any) error // Pack fields from src into properties (struct → schema)
}

// View is the shared read-only base for all named schema objects.
// It combines property access (ReadOnly) with a name identifier,
// and is embedded by both the read-only and mutable interfaces of each schema type.
type View interface {
	ReadOnly
	Name() string
}

// builtinProperties stores key-value pairs with a snapshotting (Copy-on-Write) mechanism.
// This ensures safe property passing between nodes by preventing unintended modifications
// to shared data.
type builtinProperties struct {
	entries  []entry
	readonly atomic.Bool
}

func (b *builtinProperties) Clone() *builtinProperties {
	if len(b.entries) == 0 {
		return &builtinProperties{
			entries: make([]entry, 0),
		}
	}
	newEntries := make([]entry, len(b.entries))
	copy(newEntries, b.entries)
	return &builtinProperties{
		entries: newEntries,
	}
}

func (b *builtinProperties) copyTo(dst *builtinProperties) {
	if cap(dst.entries) < len(b.entries) {
		dst.entries = make([]entry, len(b.entries))
	} else {
		dst.entries = dst.entries[:len(b.entries)]
	}
	copy(dst.entries, b.entries)
}

func (b *builtinProperties) Get(key string) (any, bool) {
	for i := len(b.entries) - 1; i >= 0; i-- {
		if b.entries[i].key == key {
			return b.entries[i].val, true
		}
	}
	return nil, false
}

func (b *builtinProperties) checkReadOnly(message string) {
	if b.readonly.Load() {
		panic(message)
	}
}

func (b *builtinProperties) isReadOnly() bool {
	return b.readonly.Load()
}

func (b *builtinProperties) setReadOnly() {
	b.readonly.Store(true)
}

func (b *builtinProperties) resetReadOnly() {
	b.readonly.Store(false)
}

func (b *builtinProperties) Bind(key string, dst any) error {
	val, ok := b.Get(key)
	if !ok {
		return ErrKeyNotFound
	}
	bs, ok := val.([]byte)
	if !ok {
		return fmt.Errorf("%w: key %q is type %T", ErrNotSerializedBytes, key, val)
	}

	return sonic.Unmarshal(bs, dst)
}

func (b *builtinProperties) Set(key string, value any) error {
	if b.readonly.Load() {
		return ErrReadOnly
	}
	dst, err := b.snapshot(value)
	if err != nil {
		return err
	}
	b.entries = append(b.entries, entry{
		key: key,
		val: dst,
	})
	return nil
}

func (b *builtinProperties) Pack(src any) error {
	if m, ok := src.(Packer); ok {
		return m.PackSchema(b)
	}
	return reflectPack(b, src)
}

func (b *builtinProperties) Unpack(dst any) error {
	if u, ok := dst.(Unpacker); ok {
		return u.UnpackSchema(b)
	}
	return reflectUnpack(b, dst)
}

// MarshalJSON serializes all properties to a JSON object.
// Entries are append-only and may contain duplicate keys; this method
// deduplicates by keeping the last value for each key (matching Get semantics).
// Values already stored as []byte (from snapshot's sonic.Marshal path) are
// embedded via sonic.RawMessage to avoid double-encoding.
func (b *builtinProperties) MarshalJSON() ([]byte, error) {
	if len(b.entries) == 0 {
		return []byte("{}"), nil
	}
	seen := make(map[string]struct{}, len(b.entries))
	m := make(map[string]any, len(b.entries))
	for i := len(b.entries) - 1; i >= 0; i-- {
		e := b.entries[i]
		if _, dup := seen[e.key]; dup {
			continue
		}
		seen[e.key] = struct{}{}
		if bs, ok := e.val.([]byte); ok {
			m[e.key] = json.RawMessage(bs)
		} else {
			m[e.key] = e.val
		}
	}
	return sonic.Marshal(m)
}

// UnmarshalJSON deserializes a JSON object into properties.
// Each key-value pair is passed through Set, which applies snapshot semantics
// to maintain COW safety.
func (b *builtinProperties) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := sonic.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, v := range m {
		if err := b.Set(k, v); err != nil {
			return fmt.Errorf("unmarshal property %s: %w", k, err)
		}
	}
	return nil
}

func (b *builtinProperties) snapshot(value any) (any, error) {
	switch v := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr:
		return v, nil
	case bool, float32, float64, string:
		return v, nil
	case time.Time, time.Duration:
		return v, nil
	case []byte:
		return buf.Clone(v), nil
	case *bytes.Buffer:
		return buf.Clone(v.Bytes()), nil
	case bytes.Buffer:
		return buf.Clone(v.Bytes()), nil
	case *strings.Builder:
		return v.String(), nil
	case strings.Builder:
		return v.String(), nil
	default:
		return sonic.Marshal(value)
	}
}
