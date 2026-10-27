package amf

import (
	"io"
)

const (
	AMF0 = 0x00
	AMF3 = 0x03
)

const (
	AMF0NumberMarker        = 0x00
	AMF0BooleanMarker       = 0x01
	AMF0StringMarker        = 0x02
	AMF0ObjectMarker        = 0x03
	AMF0MovieClipMarker     = 0x04
	AMF0NullMarker          = 0x05
	AMF0UndefinedMarker     = 0x06
	AMF0ReferenceMarker     = 0x07
	AMF0ECMAArrayMarker     = 0x08
	AMF0ObjectEndMarker     = 0x09
	AMF0StrictArrayMarker   = 0x0a
	AMF0DateMarker          = 0x0b
	AMF0LongStringMarker    = 0x0c
	AMF0UnsupportedMarker   = 0x0d
	AMF0RecordSetMarker     = 0x0e
	AMF0XMLDocumentMarker   = 0x0f
	AMF0TypedObjectMarker   = 0x10
	AMF0ACMPlusObjectMarker = 0x11
)

const (
	AMF0BooleanFalse = 0x00
	AMF0BooleanTrue  = 0x01
	AMF0StringMax    = 65535
	AMF3IntegerMax   = 536870911
)

const (
	AMF3UndefinedMarker = 0x00
	AMF3NullMarker      = 0x01
	AMF3FalseMarker     = 0x02
	AMF3TrueMarker      = 0x03
	AMF3IntegerMarker   = 0x04
	AMF3DoubleMarker    = 0x05
	AMF3StringMarker    = 0x06
	AMF3XMLDocMarker    = 0x07
	AMF3DateMarker      = 0x08
	AMF3ArrayMarker     = 0x09
	AMF3ObjectMarker    = 0x0a
	AMF3XMLStringMarker = 0x0b
	AMF3ByteArrayMarker = 0x0c
)

type ExternalHandler func(*Decoder, io.Reader) (any, error)

type Decoder struct {
	refCache         []any
	stringRefs       []string
	objectRefs       []any
	traitRefs        []Trait
	externalHandlers map[string]ExternalHandler
}

func NewDecoder() *Decoder {
	return &Decoder{
		externalHandlers: make(map[string]ExternalHandler),
	}
}

func (d *Decoder) RegisterExternalHandler(name string, f ExternalHandler) {
	d.externalHandlers[name] = f
}

type Encoder struct{}

type Version uint8

type (
	Array  []any
	Object map[string]any
)

type TypedObject struct {
	Type   string
	Object Object
}

type Trait struct {
	Type           string
	Externalizable bool
	Dynamic        bool
	Properties     []string
}

func NewTrait() *Trait {
	return &Trait{}
}

func NewTypedObject() *TypedObject {
	return &TypedObject{
		Type:   "",
		Object: make(Object),
	}
}
