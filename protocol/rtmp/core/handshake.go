package core

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ClipMyHorseTV/livego/utils/pio"
)

var timeout = 5 * time.Second

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func hsMakeDigest(key []byte, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := range 4 {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return pos
}

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return ok, digest
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return ok, digest
}

func hsCreate01(p []byte, time uint32, ver uint32, key []byte) {
	p[0] = 3
	p1 := p[1:]
	rand.Read(p1[8:])
	pio.PutU32BE(p1[0:4], time)
	pio.PutU32BE(p1[4:8], ver)
	gap := hsCalcDigestPos(p1, 8)
	digest := hsMakeDigest(key, p1, gap)
	copy(p1[gap:], digest)
}

func hsCreate2(p []byte, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := hsMakeDigest(key, p, gap)
	copy(p[gap:], digest)
}

// extendDeadline moves the connection deadline timeout into the future.
func (conn *Conn) extendDeadline() error {
	return conn.Conn.SetDeadline(time.Now().Add(timeout))
}

func (conn *Conn) HandshakeClient() error {
	var random [(1 + 1536*2) * 2]byte

	c0c1c2 := random[:1536*2+1]
	c0 := c0c1c2[:1]
	c0c1 := c0c1c2[:1536+1]

	s0s1s2 := random[1536*2+1:]

	c0[0] = 3
	// > C0C1
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := conn.rw.Write(c0c1); err != nil {
		return err
	}
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if err := conn.rw.Flush(); err != nil {
		return err
	}

	// < S0S1S2
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn.rw, s0s1s2); err != nil {
		return err
	}

	// > C2, which echoes S1
	c2 := s0s1s2[1 : 1536+1]
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := conn.rw.Write(c2); err != nil {
		return err
	}
	return conn.Conn.SetDeadline(time.Time{})
}

func (conn *Conn) HandshakeServer() error {
	var random [(1 + 1536*2) * 2]byte

	c0c1c2 := random[:1536*2+1]
	c0 := c0c1c2[:1]
	c1 := c0c1c2[1 : 1536+1]
	c0c1 := c0c1c2[:1536+1]
	c2 := c0c1c2[1536+1:]

	s0s1s2 := random[1536*2+1:]
	s0 := s0s1s2[:1]
	s1 := s0s1s2[1 : 1536+1]
	s0s1 := s0s1s2[:1536+1]
	s2 := s0s1s2[1536+1:]

	// < C0C1
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn.rw, c0c1); err != nil {
		return err
	}
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if c0[0] != 3 {
		return fmt.Errorf("rtmp: handshake version=%d invalid", c0[0])
	}

	s0[0] = 3

	clitime := pio.U32BE(c1[0:4])
	srvtime := clitime
	srvver := uint32(0x0d0e0a0d)
	cliver := pio.U32BE(c1[4:8])

	if cliver != 0 {
		ok, digest := hsParse1(c1, hsClientPartialKey, hsServerFullKey)
		if !ok {
			return errors.New("rtmp: handshake server: C1 invalid")
		}
		hsCreate01(s0s1, srvtime, srvver, hsServerPartialKey)
		hsCreate2(s2, digest)
	} else {
		copy(s1, c2)
		copy(s2, c1)
	}

	// > S0S1S2
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := conn.rw.Write(s0s1s2); err != nil {
		return err
	}
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if err := conn.rw.Flush(); err != nil {
		return err
	}

	// < C2
	if err := conn.extendDeadline(); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn.rw, c2); err != nil {
		return err
	}
	return conn.Conn.SetDeadline(time.Time{})
}
