package rtmp

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"
)

// flushConn records written chunks and reports a flush after every successful write.
type flushConn struct {
	writes  chan uint32
	flushes chan struct{}
	closed  chan struct{}
}

func newFlushConn() *flushConn {
	return &flushConn{
		writes:  make(chan uint32, 16),
		flushes: make(chan struct{}, 16),
		closed:  make(chan struct{}),
	}
}

func (c *flushConn) GetInfo() (string, string, string) {
	return "live", "room", "rtmp://host/live/room"
}
func (c *flushConn) Close(error) {}
func (c *flushConn) Write(cs core.ChunkStream) error {
	c.writes <- cs.TypeID
	return nil
}

func (c *flushConn) Flush() error {
	c.flushes <- struct{}{}
	return nil
}

// Read blocks until the test ends so VirWriter.Check does not close the writer.
func (c *flushConn) Read(*core.ChunkStream) error {
	<-c.closed
	return errors.New("closed")
}

func TestVirWriterFlushesAfterEveryWrite(t *testing.T) {
	conn := newFlushConn()
	w := NewVirWriter(conn, time.Second)
	t.Cleanup(func() {
		close(conn.closed)
		w.Close(errors.New("test done"))
	})

	packets := []*av.Packet{
		{IsMetadata: true, Data: []byte{0x02}},
		{IsVideo: true, Data: []byte{0x17, 0x00}},
		{IsAudio: true, Data: []byte{0xaf, 0x00}},
	}
	want := []uint32{av.TagScriptDataAMF0, av.TagVideo, av.TagAudio}

	for i, p := range packets {
		require.NoError(t, w.Write(p))

		select {
		case typeID := <-conn.writes:
			assert.Equal(t, want[i], typeID)
		case <-time.After(2 * time.Second):
			t.Fatalf("packet %d was not written", i)
		}
		select {
		case <-conn.flushes:
		case <-time.After(2 * time.Second):
			t.Fatalf("packet %d was written but not flushed", i)
		}
	}
}
