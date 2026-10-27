package core

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/ClipMyHorseTV/livego/utils/pio"
	"github.com/ClipMyHorseTV/livego/utils/pool"
)

const (
	_ = iota
	idSetChunkSize
	idAbortMessage
	idAck
	idUserControlMessages
	idWindowAckSize
	idSetPeerBandwidth
)

type Conn struct {
	net.Conn

	chunkSize           uint32
	remoteChunkSize     uint32
	windowAckSize       uint32
	remoteWindowAckSize uint32
	received            uint32
	ackReceived         uint32
	rw                  *ReadWriter
	chunks              map[uint32]ChunkStream
}

func NewConn(c net.Conn, bufferSize int) *Conn {
	return &Conn{
		Conn:                c,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		rw:                  NewReadWriter(c, bufferSize),
		chunks:              make(map[uint32]ChunkStream),
	}
}

func (conn *Conn) Read(c *ChunkStream) error {
	for {
		h, _ := conn.rw.ReadUintBE(1)
		format := h >> 6
		csid := h & 0x3f
		cs, ok := conn.chunks[csid]
		if !ok {
			cs = ChunkStream{}
		}
		cs.tmpFromat = format
		cs.CSID = csid

		err := cs.readChunk(conn.rw, conn.remoteChunkSize)
		if err != nil {
			return err
		}
		conn.chunks[csid] = cs

		if cs.full() {
			*c = cs
			c.Data = make([]byte, len(cs.Data))
			copy(c.Data, cs.Data)

			pool.Put(cs.Data)
			cs.Data = nil
			conn.chunks[csid] = cs

			break
		}
	}

	conn.handleControlMsg(c)
	return conn.ack(c.Length)
}

func (conn *Conn) Write(c *ChunkStream) error {
	if c.TypeID == idSetChunkSize {
		conn.chunkSize = binary.BigEndian.Uint32(c.Data)
	}
	return c.writeChunk(conn.rw, int(conn.chunkSize))
}

func (conn *Conn) Flush() error {
	return conn.rw.Flush()
}

func (conn *Conn) Close() error {
	return conn.Conn.Close()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.Conn.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.Conn.LocalAddr()
}

func (conn *Conn) SetDeadline(t time.Time) error {
	return conn.Conn.SetDeadline(t)
}

func (conn *Conn) NewAck(size uint32) ChunkStream {
	return initControlMsg(idAck, 4, size)
}

func (conn *Conn) NewSetChunkSize(size uint32) ChunkStream {
	return initControlMsg(idSetChunkSize, 4, size)
}

func (conn *Conn) NewWindowAckSize(size uint32) ChunkStream {
	return initControlMsg(idWindowAckSize, 4, size)
}

func (conn *Conn) NewSetPeerBandwidth(size uint32) ChunkStream {
	ret := initControlMsg(idSetPeerBandwidth, 5, size)
	ret.Data[4] = 2
	return ret
}

func (conn *Conn) handleControlMsg(c *ChunkStream) {
	switch c.TypeID {
	case idSetChunkSize:
		conn.remoteChunkSize = binary.BigEndian.Uint32(c.Data)
	case idWindowAckSize:
		conn.remoteWindowAckSize = binary.BigEndian.Uint32(c.Data)
	}
}

func (conn *Conn) ack(size uint32) error {
	conn.received += size
	conn.ackReceived += size
	if conn.received >= 0xf0000000 {
		conn.received = 0
	}
	if conn.ackReceived >= conn.remoteWindowAckSize {
		cs := conn.NewAck(conn.ackReceived)
		conn.ackReceived = 0
		return cs.writeChunk(conn.rw, int(conn.chunkSize))
	}
	return nil
}

func initControlMsg(id, size, value uint32) ChunkStream {
	ret := ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   id,
		StreamID: 0,
		Length:   size,
		Data:     make([]byte, size),
	}
	pio.PutU32BE(ret.Data[:size], value)
	return ret
}

const (
	streamBegin      uint32 = 0
	streamEOF        uint32 = 1
	streamDry        uint32 = 2
	setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	pingRequest      uint32 = 6
	pingResponse     uint32 = 7
)

/*
+------------------------------+-------------------------
|     Event Type ( 2- bytes )  | Event Data
+------------------------------+-------------------------
Pay load for the ‘User Control Message’.
*/
func (conn *Conn) userControlMsg(eventType, buflen uint32) ChunkStream {
	var ret ChunkStream
	buflen += 2
	ret = ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   4,
		StreamID: 1,
		Length:   buflen,
		Data:     make([]byte, buflen),
	}
	ret.Data[0] = byte(eventType >> 8 & 0xff)
	ret.Data[1] = byte(eventType & 0xff)
	return ret
}

// SetBegin sends a StreamBegin user control event for stream 1.
func (conn *Conn) SetBegin() error {
	ret := conn.userControlMsg(streamBegin, 4)
	binary.BigEndian.PutUint32(ret.Data[2:], 1)
	return conn.Write(&ret)
}

// SetRecorded sends a StreamIsRecorded user control event for stream 1.
func (conn *Conn) SetRecorded() error {
	ret := conn.userControlMsg(streamIsRecorded, 4)
	binary.BigEndian.PutUint32(ret.Data[2:], 1)
	return conn.Write(&ret)
}

// maxMessageLen is the largest message length the 24-bit RTMP message header can carry.
const maxMessageLen = 0xffffff

// MessageLength returns n as an RTMP message length, or an error if it does not fit in 24 bits.
func MessageLength(n int) (uint32, error) {
	if n < 0 || n > maxMessageLen {
		return 0, fmt.Errorf("rtmp message too large: %d bytes", n)
	}
	return uint32(n), nil
}
