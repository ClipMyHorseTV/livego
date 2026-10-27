package amf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"time"
)

// amf3 polymorphic router

func (e *Encoder) EncodeAmf3(w io.Writer, val any) (int, error) {
	if val == nil {
		return e.EncodeAmf3Null(w, true)
	}

	v := reflect.ValueOf(val)
	if !v.IsValid() {
		return e.EncodeAmf3Null(w, true)
	}

	switch v.Kind() {
	case reflect.String:
		return e.EncodeAmf3String(w, v.String(), true)
	case reflect.Bool:
		if v.Bool() {
			return e.EncodeAmf3True(w, true)
		} else {
			return e.EncodeAmf3False(w, true)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		n := v.Int()
		if n >= 0 && n <= AMF3IntegerMax {
			return e.EncodeAmf3Integer(w, uint32(n), true)
		} else {
			return e.EncodeAmf3Double(w, float64(n), true)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		n := v.Uint()
		if n <= AMF3IntegerMax {
			return e.EncodeAmf3Integer(w, uint32(n), true)
		} else {
			return e.EncodeAmf3Double(w, float64(n), true)
		}
	case reflect.Int64:
		return e.EncodeAmf3Double(w, float64(v.Int()), true)
	case reflect.Uint64:
		return e.EncodeAmf3Double(w, float64(v.Uint()), true)
	case reflect.Float32, reflect.Float64:
		return e.EncodeAmf3Double(w, float64(v.Float()), true)
	case reflect.Array, reflect.Slice:
		length := v.Len()
		arr := make(Array, length)
		for i := range length {
			arr[i] = v.Index(i).Interface()
		}
		return e.EncodeAmf3Array(w, arr, true)
	case reflect.Map:
		obj, ok := val.(Object)
		if !ok {
			return 0, errors.New("encode amf3: unable to create object from map")
		}

		to := TypedObject{}
		to.Object = obj

		return e.EncodeAmf3Object(w, to, true)
	}

	if tm, ok := val.(time.Time); ok {
		return e.EncodeAmf3Date(w, tm, true)
	}

	if to, ok := val.(TypedObject); ok {
		return e.EncodeAmf3Object(w, to, true)
	}

	return 0, fmt.Errorf("encode amf3: unsupported type %s", v.Type())
}

// EncodeAmf3Undefined encodes an AMF3 undefined value.
//
// marker: 1 byte 0x00
// no additional data.
func (e *Encoder) EncodeAmf3Undefined(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3UndefinedMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf3Null encodes an AMF3 null value.
//
// marker: 1 byte 0x01
// no additional data.
func (e *Encoder) EncodeAmf3Null(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3NullMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf3False encodes an AMF3 false value.
//
// marker: 1 byte 0x02
// no additional data.
func (e *Encoder) EncodeAmf3False(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3FalseMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf3True encodes an AMF3 true value.
//
// marker: 1 byte 0x03
// no additional data.
func (e *Encoder) EncodeAmf3True(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3TrueMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf3Integer encodes an AMF3 integer.
//
// marker: 1 byte 0x04
func (e *Encoder) EncodeAmf3Integer(w io.Writer, val uint32, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3IntegerMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	m, err = e.encodeAmf3Uint29(w, val)
	if err != nil {
		return n, err
	}
	n += m

	return n, err
}

// EncodeAmf3Double encodes an AMF3 double.
//
// marker: 1 byte 0x05
func (e *Encoder) EncodeAmf3Double(w io.Writer, val float64, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3DoubleMarker); err != nil {
			return n, err
		}
		n += 1
	}

	err = binary.Write(w, binary.BigEndian, &val)
	if err != nil {
		return n, err
	}
	n += 8

	return n, err
}

// EncodeAmf3String encodes an AMF3 string.
//
// marker: 1 byte 0x06
// format:
//   - u29 reference int. if reference, no more data. if not reference,
//     length value of bytes to read to complete string.
func (e *Encoder) EncodeAmf3String(w io.Writer, val string, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3StringMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int

	m, err = e.encodeAmf3Utf8(w, val)
	if err != nil {
		return n, err
	}
	n += m

	return n, err
}

// EncodeAmf3Date encodes an AMF3 date.
//
// marker: 1 byte 0x08
// format:
// - u29 reference int, if reference, no more data
// - timestamp double.
func (e *Encoder) EncodeAmf3Date(w io.Writer, val time.Time, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3DateMarker); err != nil {
			return n, err
		}
		n += 1
	}

	if err = WriteMarker(w, 0x01); err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode u29 for array: %w", err)
	}
	n += 1

	u64 := float64(val.Unix()) * 1000.0
	err = binary.Write(w, binary.BigEndian, &u64)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: unable to write date double: %w", err)
	}
	n += 8

	return n, err
}

// EncodeAmf3Array encodes an AMF3 array.
//
// marker: 1 byte 0x09
// format:
// - u29 reference int. if reference, no more data.
// - string representing associative array if present
// - n values (length of u29).
func (e *Encoder) EncodeAmf3Array(w io.Writer, val Array, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3ArrayMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	u29, err := amf3InlineU29(len(val))
	if err != nil {
		return n, fmt.Errorf("amf3 encode: array: %w", err)
	}

	m, err = e.encodeAmf3Uint29(w, u29)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode u29 for array: %w", err)
	}
	n += m

	m, err = e.encodeAmf3Utf8(w, "")
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode empty string for array: %w", err)
	}
	n += m

	for _, v := range val {
		m, err = e.EncodeAmf3(w, v)
		if err != nil {
			return n, fmt.Errorf("amf3 encode: cannot encode array element: %w", err)
		}
		n += m
	}

	return n, err
}

// EncodeAmf3Object encodes an AMF3 object.
//
// marker: 1 byte 0x0a
// format: ugh
func (e *Encoder) EncodeAmf3Object(w io.Writer, val TypedObject, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3ObjectMarker); err != nil {
			return n, err
		}
		n += 1
	}

	trait := *NewTrait()
	trait.Type = val.Type
	trait.Dynamic = false
	trait.Externalizable = false

	for k := range val.Object {
		trait.Properties = append(trait.Properties, k)
	}

	sort.Strings(trait.Properties)

	m, err := e.encodeAmf3Trait(w, trait)
	if err != nil {
		return n, err
	}
	n += m

	if trait.Externalizable {
		return n, errors.New("amf3 encode: cannot encode externalizable object")
	}

	for _, prop := range trait.Properties {
		m, err = e.EncodeAmf3(w, val.Object[prop])
		if err != nil {
			return n, fmt.Errorf("amf3 encode: cannot encode sealed object value: %w", err)
		}
		n += m
	}

	if trait.Dynamic {
		m, err = e.encodeAmf3DynamicMembers(w, trait, val.Object)
		n += m
	}

	return n, err
}

// encodeAmf3Trait writes an inline trait: the U29 trait header, the class name and the sealed property names.
func (e *Encoder) encodeAmf3Trait(w io.Writer, trait Trait) (n int, err error) {
	var u29 uint32 = 0x03
	if trait.Dynamic {
		u29 |= 0x02 << 2
	}

	if trait.Externalizable {
		u29 |= 0x01 << 2
	}

	// the sealed member count has to fit in the remaining 25 bits of the U29
	propCount := len(trait.Properties)
	if propCount > 0x01ffffff {
		return n, fmt.Errorf("amf3 encode: too many trait properties: %d", propCount)
	}
	u29 |= uint32(propCount) << 4

	m, err := e.encodeAmf3Uint29(w, u29)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode trait header for object: %w", err)
	}
	n += m

	m, err = e.encodeAmf3Utf8(w, trait.Type)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode trait type for object: %w", err)
	}
	n += m

	for _, prop := range trait.Properties {
		m, err = e.encodeAmf3Utf8(w, prop)
		if err != nil {
			return n, fmt.Errorf("amf3 encode: cannot encode trait property for object: %w", err)
		}
		n += m
	}

	return n, nil
}

// encodeAmf3DynamicMembers writes the members of obj that are not sealed trait properties,
// terminated by an empty string.
func (e *Encoder) encodeAmf3DynamicMembers(w io.Writer, trait Trait, obj Object) (n int, err error) {
	var m int
	for k, v := range obj {
		if slices.Contains(trait.Properties, k) {
			continue
		}

		m, err = e.encodeAmf3Utf8(w, k)
		if err != nil {
			return n, fmt.Errorf("amf3 encode: cannot encode dynamic object property key: %w", err)
		}
		n += m

		m, err = e.EncodeAmf3(w, v)
		if err != nil {
			return n, fmt.Errorf("amf3 encode: cannot encode dynamic object value: %w", err)
		}
		n += m
	}

	m, err = e.encodeAmf3Utf8(w, "")
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode dynamic object ending marker string: %w", err)
	}
	n += m

	return n, nil
}

// EncodeAmf3ByteArray encodes an AMF3 byte array.
//
// marker: 1 byte 0x0c
// format:
//   - u29 reference int. if reference, no more data. if not reference,
//     length value of bytes to read .
func (e *Encoder) EncodeAmf3ByteArray(w io.Writer, val []byte, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF3ByteArrayMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int

	u29, err := amf3InlineU29(len(val))
	if err != nil {
		return n, fmt.Errorf("amf3 encode: bytearray: %w", err)
	}

	m, err = e.encodeAmf3Uint29(w, u29)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode u29 for bytearray: %w", err)
	}
	n += m

	m, err = w.Write(val)
	if err != nil {
		return n, fmt.Errorf("encode amf3: unable to encode bytearray value: %w", err)
	}
	n += m

	return n, err
}

func (e *Encoder) encodeAmf3Utf8(w io.Writer, val string) (n int, err error) {
	u29, err := amf3InlineU29(len(val))
	if err != nil {
		return n, fmt.Errorf("amf3 encode: string: %w", err)
	}

	var m int
	m, err = e.encodeAmf3Uint29(w, u29)
	if err != nil {
		return n, fmt.Errorf("amf3 encode: cannot encode u29 for string: %w", err)
	}
	n += m

	m, err = w.Write([]byte(val))
	if err != nil {
		return n, fmt.Errorf("encode amf3: unable to encode string value: %w", err)
	}
	n += m

	return n, err
}

// amf3InlineU29 returns the U29 value (length << 1 | 1) that announces an inline value of the given length.
func amf3InlineU29(length int) (uint32, error) {
	if length < 0 || length > 0x0fffffff {
		return 0, fmt.Errorf("length %d out of u29 range", length)
	}
	return uint32(length)<<1 | 0x01, nil
}

func (e *Encoder) encodeAmf3Uint29(w io.Writer, val uint32) (n int, err error) {
	switch {
	case val <= 0x0000007F:
		err = WriteByte(w, byte(val))
		if err == nil {
			n += 1
		}
	case val <= 0x00003FFF:
		n, err = w.Write([]byte{byte((val>>7)&0x7F) | 0x80, byte(val & 0x7F)})
	case val <= 0x001FFFFF:
		n, err = w.Write([]byte{byte((val>>14)&0x7F) | 0x80, byte((val>>7)&0x7F) | 0x80, byte(val & 0x7F)})
	case val <= 0x1FFFFFFF:
		n, err = w.Write([]byte{byte((val>>22)&0x7F) | 0x80, byte((val>>15)&0x7F) | 0x80, byte((val>>8)&0x7F) | 0x80, byte(val & 0xFF)})
	default:
		return n, fmt.Errorf("amf3 encode: cannot encode u29 with value %d (out of range)", val)
	}

	return n, err
}
