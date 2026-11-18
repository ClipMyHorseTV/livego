package core

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hsSize = 1536

func serveHandshake(t *testing.T) (net.Conn, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	done := make(chan error, 1)
	go func() {
		done <- NewConn(server, 4*1024).HandshakeServer()
	}()
	return client, done
}

func TestHandshakeClientServer(t *testing.T) {
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- NewConn(serverRaw, 4*1024).HandshakeServer()
	}()

	client := NewConn(clientRaw, 4*1024)
	require.NoError(t, client.HandshakeClient())
	// HandshakeClient leaves C2 in the write buffer; the next command flushes it.
	require.NoError(t, client.rw.Flush())

	require.NoError(t, <-serverDone)
}

func TestHandshakeServerSimple(t *testing.T) {
	client, done := serveHandshake(t)

	// Version 0 in C1 selects the simple handshake: S1 echoes C2 and S2 echoes C1.
	c0c1 := make([]byte, 1+hsSize)
	c0c1[0] = 3
	_, err := rand.Read(c0c1[9:])
	require.NoError(t, err)
	_, err = client.Write(c0c1)
	require.NoError(t, err)

	s0s1s2 := make([]byte, 1+2*hsSize)
	_, err = io.ReadFull(client, s0s1s2)
	require.NoError(t, err)
	assert.Equal(t, byte(3), s0s1s2[0])
	assert.Equal(t, c0c1[1:], s0s1s2[1+hsSize:], "S2 echoes C1")

	_, err = client.Write(s0s1s2[1 : 1+hsSize])
	require.NoError(t, err)
	require.NoError(t, <-done)
}

func TestHandshakeServerDigest(t *testing.T) {
	client, done := serveHandshake(t)

	// Flash style C1 with a non-zero version and an HMAC digest.
	c0c1 := make([]byte, 1+hsSize)
	hsCreate01(c0c1, 0, 0x80000702, hsClientPartialKey)
	_, err := client.Write(c0c1)
	require.NoError(t, err)

	s0s1s2 := make([]byte, 1+2*hsSize)
	_, err = io.ReadFull(client, s0s1s2)
	require.NoError(t, err)
	assert.Equal(t, byte(3), s0s1s2[0])

	s1 := s0s1s2[1 : 1+hsSize]
	s2 := s0s1s2[1+hsSize:]

	ok, _ := hsParse1(s1, hsServerPartialKey, hsClientFullKey)
	assert.True(t, ok, "S1 must carry a digest signed with the server key")

	// S2's digest is keyed with the HMAC of the C1 digest.
	c1 := c0c1[1:]
	gap := hsCalcDigestPos(c1, 8)
	key := hsMakeDigest(hsServerFullKey, c1[gap:gap+32], -1)
	want := hsMakeDigest(key, s2, hsSize-32)
	assert.True(t, bytes.Equal(want, s2[hsSize-32:]), "S2 digest")

	_, err = client.Write(make([]byte, hsSize))
	require.NoError(t, err)
	require.NoError(t, <-done)
}

func TestHandshakeServerRejectsBadVersion(t *testing.T) {
	client, done := serveHandshake(t)

	c0c1 := make([]byte, 1+hsSize)
	c0c1[0] = 6
	_, err := client.Write(c0c1)
	require.NoError(t, err)
	require.ErrorContains(t, <-done, "version")
}

func TestHandshakeServerRejectsBadDigest(t *testing.T) {
	client, done := serveHandshake(t)

	c0c1 := make([]byte, 1+hsSize)
	hsCreate01(c0c1, 0, 0x80000702, []byte("not the flash player key"))
	_, err := client.Write(c0c1)
	require.NoError(t, err)
	require.ErrorContains(t, <-done, "C1 invalid")
}
