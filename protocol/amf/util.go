package amf

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// DumpBytes writes the first size bytes of buf to stdout as hex, for debugging.
func DumpBytes(label string, buf []byte, size int) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Dumping %s (%d bytes):\n", label, size)
	for i := range size {
		fmt.Fprintf(&sb, "0x%02x ", buf[i])
	}
	sb.WriteString("\n")
	_, _ = os.Stdout.WriteString(sb.String())
}

// Dump writes val to stdout as indented JSON, for debugging.
func Dump(label string, val any) error {
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return fmt.Errorf("error dumping %s: %w", label, err)
	}

	_, err = fmt.Fprintf(os.Stdout, "Dumping %s:\n%s\n", label, data)
	return err
}

// lenUint32 converts a length to a uint32 length field, failing if it does not fit.
func lenUint32(n int) (uint32, error) {
	if n < 0 || int64(n) > math.MaxUint32 {
		return 0, fmt.Errorf("length %d out of uint32 range", n)
	}
	return uint32(n), nil
}

func WriteByte(w io.Writer, b byte) (err error) {
	bytes := make([]byte, 1)
	bytes[0] = b

	_, err = WriteBytes(w, bytes)

	return err
}

func WriteBytes(w io.Writer, bytes []byte) (int, error) {
	return w.Write(bytes)
}

func ReadByte(r io.Reader) (byte, error) {
	bytes, err := ReadBytes(r, 1)
	if err != nil {
		return 0x00, err
	}

	return bytes[0], nil
}

func ReadBytes(r io.Reader, n int) ([]byte, error) {
	bytes := make([]byte, n)

	m, err := r.Read(bytes)
	if err != nil {
		return bytes, err
	}

	if m != n {
		return bytes, fmt.Errorf("decode read bytes failed: expected %d got %d", m, n)
	}

	return bytes, nil
}

func WriteMarker(w io.Writer, m byte) error {
	return WriteByte(w, m)
}

func ReadMarker(r io.Reader) (byte, error) {
	return ReadByte(r)
}

func AssertMarker(r io.Reader, checkMarker bool, m byte) error {
	if !checkMarker {
		return nil
	}

	marker, err := ReadMarker(r)
	if err != nil {
		return err
	}

	if marker != m {
		return fmt.Errorf("decode assert marker failed: expected %v got %v", m, marker)
	}

	return nil
}
