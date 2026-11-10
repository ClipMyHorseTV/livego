package amf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// EncodeAmf0 encodes any AMF0 value, dispatching on its type marker.
func (e *Encoder) EncodeAmf0(w io.Writer, val any) (int, error) {
	if val == nil {
		return e.EncodeAmf0Null(w, true)
	}

	v := reflect.ValueOf(val)
	if !v.IsValid() {
		return e.EncodeAmf0Null(w, true)
	}

	switch v.Kind() {
	case reflect.String:
		str := v.String()
		if len(str) <= AMF0StringMax {
			return e.EncodeAmf0String(w, str, true)
		} else {
			return e.EncodeAmf0LongString(w, str, true)
		}
	case reflect.Bool:
		return e.EncodeAmf0Boolean(w, v.Bool(), true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.EncodeAmf0Number(w, float64(v.Int()), true)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.EncodeAmf0Number(w, float64(v.Uint()), true)
	case reflect.Float32, reflect.Float64:
		return e.EncodeAmf0Number(w, float64(v.Float()), true)
	case reflect.Array, reflect.Slice:
		length := v.Len()
		arr := make(Array, length)
		for i := range length {
			arr[i] = v.Index(i).Interface()
		}
		return e.EncodeAmf0StrictArray(w, arr, true)
	case reflect.Map:
		obj, ok := val.(Object)
		if !ok {
			return 0, errors.New("encode amf0: unable to create object from map")
		}
		return e.EncodeAmf0Object(w, obj, true)
	}

	if _, ok := val.(TypedObject); ok {
		return 0, errors.New("encode amf0: unsupported type typed object")
	}

	return 0, fmt.Errorf("encode amf0: unsupported type %s", v.Type())
}

// EncodeAmf0Number encodes an AMF0 number.
//
// marker: 1 byte 0x00
// format: 8 byte big endian float64
func (e *Encoder) EncodeAmf0Number(w io.Writer, val float64, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0NumberMarker); err != nil {
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

// EncodeAmf0Boolean encodes an AMF0 boolean.
//
// marker: 1 byte 0x01
// format: 1 byte, 0x00 = false, 0x01 = true
func (e *Encoder) EncodeAmf0Boolean(w io.Writer, val bool, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0BooleanMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	buf := make([]byte, 1)
	if val {
		buf[0] = AMF0BooleanTrue
	} else {
		buf[0] = AMF0BooleanFalse
	}

	m, err = w.Write(buf)
	if err != nil {
		return n, err
	}
	n += m

	return n, err
}

// EncodeAmf0String encodes an AMF0 string.
//
// marker: 1 byte 0x02
// format:
// - 2 byte big endian uint16 header to determine size
// - n (size) byte utf8 string.
func (e *Encoder) EncodeAmf0String(w io.Writer, val string, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0StringMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	size := len(val)
	if size > AMF0StringMax {
		return n, fmt.Errorf("encode amf0: string of %d bytes is too long, use a long string", size)
	}
	length := uint16(size)
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode string length: %w", err)
	}
	n += 2

	m, err = w.Write([]byte(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode string value: %w", err)
	}
	n += m

	return n, err
}

// EncodeAmf0Object encodes an AMF0 object.
//
// marker: 1 byte 0x03
// format:
// - loop encoded string followed by encoded value
// - terminated with empty string followed by 1 byte 0x09.
func (e *Encoder) EncodeAmf0Object(w io.Writer, val Object, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0ObjectMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	for k, v := range val {
		m, err = e.EncodeAmf0String(w, k, false)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode object key: %w", err)
		}
		n += m

		m, err = e.EncodeAmf0(w, v)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode object value: %w", err)
		}
		n += m
	}

	m, err = e.EncodeAmf0String(w, "", false)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode object empty string: %w", err)
	}
	n += m

	err = WriteMarker(w, AMF0ObjectEndMarker)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to object end marker: %w", err)
	}
	n += 1

	return n, err
}

// EncodeAmf0Null encodes an AMF0 null value.
//
// marker: 1 byte 0x05
// no additional data.
func (e *Encoder) EncodeAmf0Null(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0NullMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf0Undefined encodes an AMF0 undefined value.
//
// marker: 1 byte 0x06
// no additional data.
func (e *Encoder) EncodeAmf0Undefined(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0UndefinedMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf0EcmaArray encodes an AMF0 ECMA array.
//
// marker: 1 byte 0x08
// format:
// - 4 byte big endian uint32 with length of associative array
// - normal object format:
//   - loop encoded string followed by encoded value
//   - terminated with empty string followed by 1 byte 0x09
func (e *Encoder) EncodeAmf0EcmaArray(w io.Writer, val Object, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0ECMAArrayMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	length, err := lenUint32(len(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: ecma array: %w", err)
	}
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode ecma array length: %w", err)
	}
	n += 4

	m, err = e.EncodeAmf0Object(w, val, false)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode ecma array object: %w", err)
	}
	n += m

	return n, err
}

// EncodeAmf0StrictArray encodes an AMF0 strict array.
//
// marker: 1 byte 0x0a
// format:
// - 4 byte big endian uint32 to determine length of associative array
// - n (length) encoded values.
func (e *Encoder) EncodeAmf0StrictArray(w io.Writer, val Array, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0StrictArrayMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	length, err := lenUint32(len(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: strict array: %w", err)
	}
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode strict array length: %w", err)
	}
	n += 4

	for _, v := range val {
		m, err = e.EncodeAmf0(w, v)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode strict array element: %w", err)
		}
		n += m
	}

	return n, err
}

// EncodeAmf0LongString encodes an AMF0 long string.
//
// marker: 1 byte 0x0c
// format:
// - 4 byte big endian uint32 header to determine size
// - n (size) byte utf8 string.
func (e *Encoder) EncodeAmf0LongString(w io.Writer, val string, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0LongStringMarker); err != nil {
			return n, err
		}
		n += 1
	}

	var m int
	length, err := lenUint32(len(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: long string: %w", err)
	}
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode long string length: %w", err)
	}
	n += 4

	m, err = w.Write([]byte(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode long string value: %w", err)
	}
	n += m

	return n, err
}

// EncodeAmf0Unsupported encodes an AMF0 unsupported value.
//
// marker: 1 byte 0x0d
// no additional data.
func (e *Encoder) EncodeAmf0Unsupported(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0UnsupportedMarker); err != nil {
			return n, err
		}
		n += 1
	}

	return n, err
}

// EncodeAmf0Amf3Marker writes the AMF0 marker that switches to AMF3 encoding.
//
// marker: 1 byte 0x11
func (e *Encoder) EncodeAmf0Amf3Marker(w io.Writer) error {
	return WriteMarker(w, AMF0ACMPlusObjectMarker)
}
