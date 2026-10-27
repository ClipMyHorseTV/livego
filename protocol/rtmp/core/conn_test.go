package core

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnReadNormal(t *testing.T) {
	at := assert.New(t)
	data := []byte{
		0x06, 0x00, 0x00, 0x00, 0x00, 0x01, 0x33, 0x09, 0x01, 0x00, 0x00, 0x00,
	}
	data1 := make([]byte, 128)
	data2 := make([]byte, 51)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data2...)
	conn := &Conn{
		rw:                  NewReadWriter(bytes.NewBuffer(data), 1024),
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		chunks:              make(map[uint32]ChunkStream),
	}
	var c ChunkStream
	err := conn.Read(&c)
	at.NoError(err)
	at.Equal(6, int(c.CSID))
	at.Equal(307, int(c.Length))
	at.Equal(9, int(c.TypeID))
}

// 交叉读音视频数据.
func TestConnCrossReading(t *testing.T) {
	at := assert.New(t)
	data1 := make([]byte, 128)
	data2 := make([]byte, 51)

	videoData := []byte{
		0x06, 0x00, 0x00, 0x00, 0x00, 0x01, 0x33, 0x09, 0x01, 0x00, 0x00, 0x00,
	}
	audioData := []byte{
		0x04, 0x00, 0x00, 0x00, 0x00, 0x01, 0x33, 0x08, 0x01, 0x00, 0x00, 0x00,
	}
	// video 1
	videoData = append(videoData, data1...)
	// video 2
	videoData = append(videoData, 0xc6)
	videoData = append(videoData, data1...)
	// audio 1
	videoData = append(videoData, audioData...)
	videoData = append(videoData, data1...)
	// audio 2
	videoData = append(videoData, 0xc4)
	videoData = append(videoData, data1...)
	// video 3
	videoData = append(videoData, 0xc6)
	videoData = append(videoData, data2...)
	// audio 3
	videoData = append(videoData, 0xc4)
	videoData = append(videoData, data2...)

	conn := &Conn{
		rw:                  NewReadWriter(bytes.NewBuffer(videoData), 1024),
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		chunks:              make(map[uint32]ChunkStream),
	}
	var c ChunkStream
	// video 1
	err := conn.Read(&c)
	at.NoError(err)
	at.Equal(9, int(c.TypeID))
	at.Equal(307, len(c.Data))

	// audio2
	err = conn.Read(&c)
	at.NoError(err)
	at.Equal(8, int(c.TypeID))
	at.Equal(307, len(c.Data))

	err = conn.Read(&c)
	at.Equal(err, io.EOF)
}

func TestSetChunksizeForWrite(t *testing.T) {
	at := assert.New(t)
	chunk := ChunkStream{
		Format:    0,
		CSID:      2,
		Timestamp: 0,
		Length:    4,
		StreamID:  1,
		TypeID:    idSetChunkSize,
		Data:      []byte{0x00, 0x00, 0x00, 0x96},
	}
	buf := bytes.NewBuffer(nil)
	rw := NewReadWriter(buf, 1024)
	conn := &Conn{
		rw:                  rw,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		chunks:              make(map[uint32]ChunkStream),
	}

	audio := ChunkStream{
		Format:    0,
		CSID:      4,
		Timestamp: 40,
		Length:    133,
		StreamID:  1,
		TypeID:    0x8,
	}
	audio.Data = make([]byte, 133)
	audio.Data = audio.Data[:133]
	audio.Data[0] = 0xff
	audio.Data[128] = 0xff
	err := conn.Write(&audio)
	at.NoError(err)
	conn.Flush()
	at.Equal(146, len(buf.Bytes()))

	buf.Reset()
	err = conn.Write(&chunk)
	at.NoError(err)
	conn.Flush()

	buf.Reset()
	err = conn.Write(&audio)
	at.NoError(err)
	conn.Flush()
	at.Equal(145, len(buf.Bytes()))
}

func TestSetChunksize(t *testing.T) {
	at := assert.New(t)
	data := []byte{
		0x06, 0x00, 0x00, 0x00, 0x00, 0x01, 0x33, 0x09, 0x01, 0x00, 0x00, 0x00,
	}
	data1 := make([]byte, 128)
	data2 := make([]byte, 51)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data2...)
	rw := NewReadWriter(bytes.NewBuffer(data), 1024)
	conn := &Conn{
		rw:                  rw,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		chunks:              make(map[uint32]ChunkStream),
	}

	var c ChunkStream
	err := conn.Read(&c)
	at.NoError(err)
	at.Equal(9, int(c.TypeID))
	at.Equal(6, int(c.CSID))
	at.Equal(1, int(c.StreamID))
	at.Equal(307, len(c.Data))

	// 设置chunksize
	chunkBuf := []byte{
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x96,
	}
	conn.rw = NewReadWriter(bytes.NewBuffer(chunkBuf), 1024)
	err = conn.Read(&c)
	at.NoError(err)

	data = data[:12]
	data[7] = 0x8
	data1 = make([]byte, 150)
	data2 = make([]byte, 7)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data1...)
	data = append(data, 0xc6)
	data = append(data, data2...)

	conn.rw = NewReadWriter(bytes.NewBuffer(data), 1024)
	err = conn.Read(&c)
	at.NoError(err)
	at.Equal(307, len(c.Data))

	err = conn.Read(&c)
	at.Equal(err, io.EOF)
}

func TestConnWrite(t *testing.T) {
	at := assert.New(t)
	wr := bytes.NewBuffer(nil)
	readWriter := NewReadWriter(wr, 128)
	conn := &Conn{
		rw:                  readWriter,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		chunks:              make(map[uint32]ChunkStream),
	}

	c1 := ChunkStream{
		Length:    3,
		TypeID:    8,
		CSID:      3,
		Timestamp: 40,
		Data:      []byte{0x01, 0x02, 0x03},
	}
	err := conn.Write(&c1)
	at.NoError(err)
	conn.Flush()
	at.Equal([]byte{0x4, 0x0, 0x0, 0x28, 0x0, 0x0, 0x3, 0x8, 0x0, 0x0, 0x0, 0x0, 0x1, 0x2, 0x3}, wr.Bytes())

	// for type 1
	wr.Reset()
	c1 = ChunkStream{
		Length:    4,
		TypeID:    8,
		CSID:      3,
		Timestamp: 80,
		Data:      []byte{0x01, 0x02, 0x03, 0x4},
	}
	err = conn.Write(&c1)
	at.NoError(err)
	conn.Flush()
	at.Equal([]byte{0x4, 0x0, 0x0, 0x50, 0x0, 0x0, 0x4, 0x8, 0x0, 0x0, 0x0, 0x0, 0x1, 0x2, 0x3, 0x4}, wr.Bytes())

	// for type 2
	wr.Reset()
	c1.Timestamp = 160
	err = conn.Write(&c1)
	at.NoError(err)
	conn.Flush()
	at.Equal([]byte{0x4, 0x0, 0x0, 0xa0, 0x0, 0x0, 0x4, 0x8, 0x0, 0x0, 0x0, 0x0, 0x1, 0x2, 0x3, 0x4}, wr.Bytes())
}
