package pio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsignedRoundTrip(t *testing.T) {
	b := make([]byte, 8)

	PutU16BE(b, 0xbeef)
	assert.Equal(t, []byte{0xbe, 0xef}, b[:2])
	assert.Equal(t, uint16(0xbeef), U16BE(b))

	PutU24BE(b, 0xabcdef)
	assert.Equal(t, []byte{0xab, 0xcd, 0xef}, b[:3])
	assert.Equal(t, uint32(0xabcdef), U24BE(b))

	PutU32BE(b, 0xdeadbeef)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, b[:4])
	assert.Equal(t, uint32(0xdeadbeef), U32BE(b))

	PutU32LE(b, 0xdeadbeef)
	assert.Equal(t, []byte{0xef, 0xbe, 0xad, 0xde}, b[:4])
	assert.Equal(t, uint32(0xdeadbeef), U32LE(b))

	PutU40BE(b, 0x0102030405)
	assert.Equal(t, uint64(0x0102030405), U40BE(b))

	PutU64BE(b, 0x0102030405060708)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, b)
	assert.Equal(t, uint64(0x0102030405060708), U64BE(b))

	PutU48BE(b, 0x0a0b0c0d0e0f)
	assert.Equal(t, []byte{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, b[:6])
}

func TestSignedRoundTrip(t *testing.T) {
	b := make([]byte, 8)

	for _, v := range []int16{0, 1, -1, 32767, -32768} {
		PutI16BE(b, v)
		assert.Equal(t, v, I16BE(b))
	}

	// 24 bit values must be sign extended, FLV composition times rely on this.
	for _, v := range []int32{0, 1, -1, -40, 0x7fffff, -0x800000} {
		PutI24BE(b, v)
		assert.Equal(t, v, I24BE(b), "I24BE(%d)", v)
	}

	for _, v := range []int32{0, -1, 0x7fffffff, -0x80000000} {
		PutI32BE(b, v)
		assert.Equal(t, v, I32BE(b))
	}

	for _, v := range []int64{0, -1, 0x7fffffffffffffff, -0x8000000000000000} {
		PutI64BE(b, v)
		assert.Equal(t, v, I64BE(b))
	}
}
