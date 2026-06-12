package schema

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
)

// Packer is implemented by types that can pack their fields
// into schema Properties (struct → schema).
type Packer interface {
	PackSchema(props Properties) error
}

// Unpacker is implemented by types that can unpack schema
// properties into their own fields (schema → struct).
type Unpacker interface {
	UnpackSchema(data ReadOnly) error
}

// tag options
const (
	tagName      = "schema"
	tagOmitEmpty = "omitempty"
	tagOmitZero  = "omitzero"
	tagRequired  = "required"
	tagSkip      = "-"
)

type fieldFlag uint8

const (
	flagRequired  fieldFlag = 1 << iota // required on unpack
	flagOmitEmpty                       // omit zero-value on pack
	flagOmitZero                        // omit zero-value and empty collections on pack
)

type fieldInfo struct {
	index int
	key   string
	flags fieldFlag
}

func (f fieldInfo) has(flag fieldFlag) bool { return f.flags&flag != 0 }

type typeInfo struct {
	fields []fieldInfo
}

var typeCache sync.Map // map[reflect.Type]*typeInfo

func cachedTypeInfo(rt reflect.Type) *typeInfo {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if cached, ok := typeCache.Load(rt); ok {
		return cached.(*typeInfo)
	}
	info := buildTypeInfo(rt)
	actual, _ := typeCache.LoadOrStore(rt, info)
	return actual.(*typeInfo)
}

func buildTypeInfo(rt reflect.Type) *typeInfo {
	info := &typeInfo{}
	for i := range rt.NumField() {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get(tagName)
		if tag == "" || tag == tagSkip {
			continue
		}
		parts := strings.Split(tag, ",")
		fi := fieldInfo{
			index: i,
			key:   parts[0],
		}
		for _, opt := range parts[1:] {
			switch opt {
			case tagRequired:
				fi.flags |= flagRequired
			case tagOmitEmpty:
				fi.flags |= flagOmitEmpty
			case tagOmitZero:
				fi.flags |= flagOmitZero
			}
		}
		info.fields = append(info.fields, fi)
	}
	return info
}

// reflectPack uses struct tags to pack fields into Properties.
func reflectPack(props Properties, src any) error {
	rv := reflect.ValueOf(src)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("schema: pack source is nil pointer")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("schema: pack source must be a struct, got %s", rv.Kind())
	}
	info := cachedTypeInfo(rv.Type())
	for _, fi := range info.fields {
		fv := rv.Field(fi.index)
		if fi.has(flagOmitZero) && isEmpty(fv) {
			continue
		}
		if fi.has(flagOmitEmpty) && fv.IsZero() {
			continue
		}
		if err := props.Set(fi.key, fv.Interface()); err != nil {
			return fmt.Errorf("schema: pack field %q (key %q): %w",
				rv.Type().Field(fi.index).Name, fi.key, err)
		}
	}
	return nil
}

// reflectUnpack uses struct tags to unpack Properties into a struct.
func reflectUnpack(data ReadOnly, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("schema: unpack destination must be a non-nil pointer to struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("schema: unpack destination must be a pointer to struct, got pointer to %s", rv.Kind())
	}
	info := cachedTypeInfo(rv.Type())
	for _, fi := range info.fields {
		val, ok := data.Get(fi.key)
		if !ok {
			if fi.has(flagRequired) {
				return fmt.Errorf("schema: required key %q not found", fi.key)
			}
			continue
		}
		fv := rv.Field(fi.index)
		if err := assignField(fv, val); err != nil {
			return fmt.Errorf("schema: unpack key %q into field %q: %w",
				fi.key, rv.Type().Field(fi.index).Name, err)
		}
	}
	return nil
}

// assignField sets the struct field value from a schema property value.
func assignField(fv reflect.Value, val any) error {
	rv := reflect.ValueOf(val)
	// Direct assignment if types are compatible
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return nil
	}
	if rv.Type().ConvertibleTo(fv.Type()) {
		fv.Set(rv.Convert(fv.Type()))
		return nil
	}
	// snapshot stores complex types as []byte via sonic.Marshal,
	// try to unmarshal back into the target type.
	if bs, ok := val.([]byte); ok {
		dst := reflect.New(fv.Type()).Interface()
		if err := sonic.Unmarshal(bs, dst); err != nil {
			return fmt.Errorf("%w: cannot unmarshal []byte into %s: %w", ErrTypeMismatch, fv.Type(), err)
		}
		fv.Set(reflect.ValueOf(dst).Elem())
		return nil
	}
	return fmt.Errorf("%w: cannot assign %T to %s", ErrTypeMismatch, val, fv.Type())
}

// isEmpty reports whether a value is logically empty:
// zero value for scalars, nil or zero-length for collections.
func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Array:
		return v.Len() == 0
	case reflect.String:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
