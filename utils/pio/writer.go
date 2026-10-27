package pio

func PutU8(b []byte, v uint8) {
	b[0] = v
}

func PutI16BE(b []byte, v int16) {
	b[0] = byte((v >> 8) & 0xff)
	b[1] = byte(v & 0xff)
}

func PutU16BE(b []byte, v uint16) {
	b[0] = byte((v >> 8) & 0xff)
	b[1] = byte(v & 0xff)
}

func PutI24BE(b []byte, v int32) {
	b[0] = byte((v >> 16) & 0xff)
	b[1] = byte((v >> 8) & 0xff)
	b[2] = byte(v & 0xff)
}

func PutU24BE(b []byte, v uint32) {
	b[0] = byte((v >> 16) & 0xff)
	b[1] = byte((v >> 8) & 0xff)
	b[2] = byte(v & 0xff)
}

func PutI32BE(b []byte, v int32) {
	b[0] = byte((v >> 24) & 0xff)
	b[1] = byte((v >> 16) & 0xff)
	b[2] = byte((v >> 8) & 0xff)
	b[3] = byte(v & 0xff)
}

func PutU32BE(b []byte, v uint32) {
	b[0] = byte((v >> 24) & 0xff)
	b[1] = byte((v >> 16) & 0xff)
	b[2] = byte((v >> 8) & 0xff)
	b[3] = byte(v & 0xff)
}

func PutU32LE(b []byte, v uint32) {
	b[3] = byte((v >> 24) & 0xff)
	b[2] = byte((v >> 16) & 0xff)
	b[1] = byte((v >> 8) & 0xff)
	b[0] = byte(v & 0xff)
}

func PutU40BE(b []byte, v uint64) {
	b[0] = byte((v >> 32) & 0xff)
	b[1] = byte((v >> 24) & 0xff)
	b[2] = byte((v >> 16) & 0xff)
	b[3] = byte((v >> 8) & 0xff)
	b[4] = byte(v & 0xff)
}

func PutU48BE(b []byte, v uint64) {
	b[0] = byte((v >> 40) & 0xff)
	b[1] = byte((v >> 32) & 0xff)
	b[2] = byte((v >> 24) & 0xff)
	b[3] = byte((v >> 16) & 0xff)
	b[4] = byte((v >> 8) & 0xff)
	b[5] = byte(v & 0xff)
}

func PutU64BE(b []byte, v uint64) {
	b[0] = byte((v >> 56) & 0xff)
	b[1] = byte((v >> 48) & 0xff)
	b[2] = byte((v >> 40) & 0xff)
	b[3] = byte((v >> 32) & 0xff)
	b[4] = byte((v >> 24) & 0xff)
	b[5] = byte((v >> 16) & 0xff)
	b[6] = byte((v >> 8) & 0xff)
	b[7] = byte(v & 0xff)
}

func PutI64BE(b []byte, v int64) {
	b[0] = byte((v >> 56) & 0xff)
	b[1] = byte((v >> 48) & 0xff)
	b[2] = byte((v >> 40) & 0xff)
	b[3] = byte((v >> 32) & 0xff)
	b[4] = byte((v >> 24) & 0xff)
	b[5] = byte((v >> 16) & 0xff)
	b[6] = byte((v >> 8) & 0xff)
	b[7] = byte(v & 0xff)
}
