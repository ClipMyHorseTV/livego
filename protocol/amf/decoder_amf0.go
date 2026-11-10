package amf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DecodeAmf0 decodes any AMF0 value, dispatching on its type marker.
func (d *Decoder) DecodeAmf0(r io.Reader) (any, error) {
	marker, err := ReadMarker(r)
	if err != nil {
		return nil, err
	}

	switch marker {
	case AMF0NumberMarker:
		return d.DecodeAmf0Number(r, false)
	case AMF0BooleanMarker:
		return d.DecodeAmf0Boolean(r, false)
	case AMF0StringMarker:
		return d.DecodeAmf0String(r, false)
	case AMF0ObjectMarker:
		return d.DecodeAmf0Object(r, false)
	case AMF0MovieClipMarker:
		return nil, errors.New("decode amf0: unsupported type movieclip")
	case AMF0NullMarker:
		return d.DecodeAmf0Null(r, false)
	case AMF0UndefinedMarker:
		return d.DecodeAmf0Undefined(r, false)
	case AMF0ReferenceMarker:
		return nil, errors.New("decode amf0: unsupported type reference")
	case AMF0ECMAArrayMarker:
		return d.DecodeAmf0EcmaArray(r, false)
	case AMF0StrictArrayMarker:
		return d.DecodeAmf0StrictArray(r, false)
	case AMF0DateMarker:
		return d.DecodeAmf0Date(r, false)
	case AMF0LongStringMarker:
		return d.DecodeAmf0LongString(r, false)
	case AMF0UnsupportedMarker:
		return d.DecodeAmf0Unsupported(r, false)
	case AMF0RecordSetMarker:
		return nil, errors.New("decode amf0: unsupported type recordset")
	case AMF0XMLDocumentMarker:
		return d.DecodeAmf0XmlDocument(r, false)
	case AMF0TypedObjectMarker:
		return d.DecodeAmf0TypedObject(r, false)
	case AMF0ACMPlusObjectMarker:
		return d.DecodeAmf3(r)
	}

	return nil, fmt.Errorf("decode amf0: unsupported type %d", marker)
}

// DecodeAmf0Number decodes an AMF0 number.
//
// marker: 1 byte 0x00
// format: 8 byte big endian float64
func (d *Decoder) DecodeAmf0Number(r io.Reader, decodeMarker bool) (result float64, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0NumberMarker); err != nil {
		return result, err
	}

	err = binary.Read(r, binary.BigEndian, &result)
	if err != nil {
		return float64(0), fmt.Errorf("amf0 decode: unable to read number: %w", err)
	}

	return result, err
}

// DecodeAmf0Boolean decodes an AMF0 boolean.
//
// marker: 1 byte 0x01
// format: 1 byte, 0x00 = false, 0x01 = true
func (d *Decoder) DecodeAmf0Boolean(r io.Reader, decodeMarker bool) (result bool, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0BooleanMarker); err != nil {
		return result, err
	}

	var b byte
	if b, err = ReadByte(r); err != nil {
		return result, err
	}

	switch b {
	case AMF0BooleanFalse:
		return false, nil
	case AMF0BooleanTrue:
		return true, nil
	}

	return false, fmt.Errorf("decode amf0: unexpected value %v for boolean", b)
}

// DecodeAmf0String decodes an AMF0 string.
//
// marker: 1 byte 0x02
// format:
// - 2 byte big endian uint16 header to determine size
// - n (size) byte utf8 string.
func (d *Decoder) DecodeAmf0String(r io.Reader, decodeMarker bool) (result string, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0StringMarker); err != nil {
		return result, err
	}

	var length uint16
	err = binary.Read(r, binary.BigEndian, &length)
	if err != nil {
		return "", fmt.Errorf("decode amf0: unable to decode string length: %w", err)
	}

	data, err := ReadBytes(r, int(length))
	if err != nil {
		return "", fmt.Errorf("decode amf0: unable to decode string value: %w", err)
	}

	return string(data), nil
}

// DecodeAmf0Object decodes an AMF0 object.
//
// marker: 1 byte 0x03
// format:
// - loop encoded string followed by encoded value
// - terminated with empty string followed by 1 byte 0x09.
func (d *Decoder) DecodeAmf0Object(r io.Reader, decodeMarker bool) (Object, error) {
	if err := AssertMarker(r, decodeMarker, AMF0ObjectMarker); err != nil {
		return nil, err
	}

	result := make(Object)
	d.refCache = append(d.refCache, result)

	for {
		key, err := d.DecodeAmf0String(r, false)
		if err != nil {
			return nil, err
		}

		if key == "" {
			if err = AssertMarker(r, true, AMF0ObjectEndMarker); err != nil {
				return nil, fmt.Errorf("decode amf0: expected object end marker: %w", err)
			}

			break
		}

		value, err := d.DecodeAmf0(r)
		if err != nil {
			return nil, fmt.Errorf("decode amf0: unable to decode object value: %w", err)
		}

		result[key] = value
	}

	return result, nil
}

// DecodeAmf0Null decodes an AMF0 null value.
//
// marker: 1 byte 0x05
// no additional data.
func (d *Decoder) DecodeAmf0Null(r io.Reader, decodeMarker bool) (result any, err error) {
	err = AssertMarker(r, decodeMarker, AMF0NullMarker)
	return result, err
}

// DecodeAmf0Undefined decodes an AMF0 undefined value.
//
// marker: 1 byte 0x06
// no additional data.
func (d *Decoder) DecodeAmf0Undefined(r io.Reader, decodeMarker bool) (result any, err error) {
	err = AssertMarker(r, decodeMarker, AMF0UndefinedMarker)
	return result, err
}

// marker: 1 byte 0x07
// format: 2 byte big endian uint16
/*
func (d *Decoder) DecodeAmf0Reference(r io.Reader, decodeMarker bool) (interface{}, error) {
	if err := AssertMarker(r, decodeMarker, AMF0_REFERENCE_MARKER); err != nil {
		return nil, err
	}

	var err error
	var ref uint16

	err = binary.Read(r, binary.BigEndian, &ref)
	if err != nil {
		return nil, fmt.Errorf("decode amf0: unable to decode reference id: %s", err)
	}

	if int(ref) > len(d.refCache) {
		return nil, fmt.Errorf("decode amf0: bad reference %d (current length %d)", ref, len(d.refCache))
	}

	result := d.refCache[ref]

	return result, nil
}
*/

// DecodeAmf0EcmaArray decodes an AMF0 ECMA array.
//
// marker: 1 byte 0x08
// format:
// - 4 byte big endian uint32 with length of associative array
// - normal object format:
//   - loop encoded string followed by encoded value
//   - terminated with empty string followed by 1 byte 0x09
func (d *Decoder) DecodeAmf0EcmaArray(r io.Reader, decodeMarker bool) (Object, error) {
	if err := AssertMarker(r, decodeMarker, AMF0ECMAArrayMarker); err != nil {
		return nil, err
	}

	// The associative-count is only a hint; the object is terminated by the end marker.
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("decode amf0: unable to decode ecma array length: %w", err)
	}

	result, err := d.DecodeAmf0Object(r, false)
	if err != nil {
		return nil, fmt.Errorf("decode amf0: unable to decode ecma array object: %w", err)
	}

	return result, nil
}

// DecodeAmf0StrictArray decodes an AMF0 strict array.
//
// marker: 1 byte 0x0a
// format:
// - 4 byte big endian uint32 to determine length of associative array
// - n (length) encoded values.
func (d *Decoder) DecodeAmf0StrictArray(r io.Reader, decodeMarker bool) (result Array, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0StrictArrayMarker); err != nil {
		return nil, err
	}

	var length uint32
	err = binary.Read(r, binary.BigEndian, &length)
	if err != nil {
		return nil, fmt.Errorf("decode amf0: unable to decode strict array length: %w", err)
	}

	d.refCache = append(d.refCache, result)

	for range length {
		tmp, err := d.DecodeAmf0(r)
		if err != nil {
			return nil, fmt.Errorf("decode amf0: unable to decode strict array object: %w", err)
		}
		result = append(result, tmp)
	}

	return result, nil
}

// DecodeAmf0Date decodes an AMF0 date.
//
// marker: 1 byte 0x0b
// format:
// - normal number format:
//   - 8 byte big endian float64
//
// - 2 byte unused.
func (d *Decoder) DecodeAmf0Date(r io.Reader, decodeMarker bool) (result float64, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0DateMarker); err != nil {
		return result, err
	}

	if result, err = d.DecodeAmf0Number(r, false); err != nil {
		return float64(0), fmt.Errorf("decode amf0: unable to decode float in date: %w", err)
	}

	if _, err = ReadBytes(r, 2); err != nil {
		return float64(0), fmt.Errorf("decode amf0: unable to read 2 trail bytes in date: %w", err)
	}

	return result, err
}

// DecodeAmf0LongString decodes an AMF0 long string.
//
// marker: 1 byte 0x0c
// format:
// - 4 byte big endian uint32 header to determine size
// - n (size) byte utf8 string.
func (d *Decoder) DecodeAmf0LongString(r io.Reader, decodeMarker bool) (result string, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0LongStringMarker); err != nil {
		return result, err
	}

	var length uint32
	err = binary.Read(r, binary.BigEndian, &length)
	if err != nil {
		return "", fmt.Errorf("decode amf0: unable to decode long string length: %w", err)
	}

	data, err := ReadBytes(r, int(length))
	if err != nil {
		return "", fmt.Errorf("decode amf0: unable to decode long string value: %w", err)
	}

	return string(data), nil
}

// DecodeAmf0Unsupported decodes an AMF0 unsupported value.
//
// marker: 1 byte 0x0d
// no additional data.
func (d *Decoder) DecodeAmf0Unsupported(r io.Reader, decodeMarker bool) (result any, err error) {
	err = AssertMarker(r, decodeMarker, AMF0UnsupportedMarker)
	return result, err
}

// DecodeAmf0XmlDocument decodes an AMF0 XML document.
//
// marker: 1 byte 0x0f
// format:
// - normal long string format
//   - 4 byte big endian uint32 header to determine size
//   - n (size) byte utf8 string
func (d *Decoder) DecodeAmf0XmlDocument(r io.Reader, decodeMarker bool) (result string, err error) {
	if err = AssertMarker(r, decodeMarker, AMF0XMLDocumentMarker); err != nil {
		return result, err
	}

	return d.DecodeAmf0LongString(r, false)
}

// DecodeAmf0TypedObject decodes an AMF0 typed object.
//
// marker: 1 byte 0x10
// format:
// - normal string format:
//   - 2 byte big endian uint16 header to determine size
//   - n (size) byte utf8 string
//
// - normal object format:
//   - loop encoded string followed by encoded value
//   - terminated with empty string followed by 1 byte 0x09
func (d *Decoder) DecodeAmf0TypedObject(r io.Reader, decodeMarker bool) (TypedObject, error) {
	result := TypedObject{}

	err := AssertMarker(r, decodeMarker, AMF0TypedObjectMarker)
	if err != nil {
		return result, err
	}

	d.refCache = append(d.refCache, result)

	result.Type, err = d.DecodeAmf0String(r, false)
	if err != nil {
		return result, fmt.Errorf("decode amf0: typed object unable to determine type: %w", err)
	}

	result.Object, err = d.DecodeAmf0Object(r, false)
	if err != nil {
		return result, fmt.Errorf("decode amf0: typed object unable to determine object: %w", err)
	}

	return result, nil
}
