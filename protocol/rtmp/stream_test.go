package rtmp

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/container/flv"
)

// fakeReader is a publisher fed through a channel; closing the channel ends the stream.
type fakeReader struct {
	uid     string
	packets chan av.Packet
	closed  chan struct{}
	once    sync.Once
}

func newFakeReader(uid string) *fakeReader {
	return &fakeReader{uid: uid, packets: make(chan av.Packet), closed: make(chan struct{})}
}

func (r *fakeReader) Read(p *av.Packet) error {
	select {
	case pkt, ok := <-r.packets:
		if !ok {
			return io.EOF
		}
		*p = pkt
		return nil
	case <-r.closed:
		return errors.New("closed")
	}
}
func (r *fakeReader) Info() av.Info { return av.Info{Key: "live/room", UID: r.uid} }
func (r *fakeReader) Alive() bool   { return true }
func (r *fakeReader) Close(error)   { r.once.Do(func() { close(r.closed) }) }

// fakeWriter is a player that records what it receives.
type fakeWriter struct {
	uid  string
	fail bool

	mu       sync.Mutex
	received []uint32
	closed   chan struct{}
	once     sync.Once
}

func newFakeWriter(uid string) *fakeWriter {
	return &fakeWriter{uid: uid, closed: make(chan struct{})}
}

func (w *fakeWriter) Write(p *av.Packet) error {
	if w.fail {
		return errors.New("broken pipe")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.received = append(w.received, p.TimeStamp)
	return nil
}

func (w *fakeWriter) timestamps() []uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]uint32(nil), w.received...)
}
func (w *fakeWriter) Info() av.Info      { return av.Info{Key: "live/room", UID: w.uid} }
func (w *fakeWriter) Alive() bool        { return true }
func (w *fakeWriter) CalcBaseTimestamp() {}
func (w *fakeWriter) Close(error)        { w.once.Do(func() { close(w.closed) }) }

func video(t *testing.T, ts uint32, data ...byte) av.Packet {
	t.Helper()
	p := av.Packet{IsVideo: true, TimeStamp: ts, Data: data}
	require.NoError(t, flv.NewDemuxer().DemuxH(&p))
	return p
}

func waitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for close")
	}
}

func TestStreamFanOut(t *testing.T) {
	cfg := DefaultConfig()
	s := NewStream(&cfg)
	s.info = av.Info{Key: "live/room"}

	early := newFakeWriter("early")
	s.AddWriter(early)

	r := newFakeReader("pub")
	s.AddReader(r)

	r.packets <- video(t, 1, 0x17, 0x00, 0, 0, 0) // sequence header
	r.packets <- video(t, 2, 0x17, 0x01, 0, 0, 0) // key frame
	r.packets <- video(t, 3, 0x27, 0x01, 0, 0, 0) // inter frame

	// A player joining mid-GOP must first get the sequence header and the GOP so far.
	late := newFakeWriter("late")
	s.AddWriter(late)
	r.packets <- video(t, 4, 0x27, 0x01, 0, 0, 0)

	close(r.packets)
	waitClosed(t, early.closed)
	waitClosed(t, late.closed)

	assert.Equal(t, []uint32{1, 2, 3, 4}, early.timestamps())
	assert.Equal(t, []uint32{1, 2, 3, 4}, late.timestamps(), "cache replay already contains the current packet, no duplicate")
}

func TestStreamRemovesFailingWriter(t *testing.T) {
	cfg := DefaultConfig()
	s := NewStream(&cfg)
	s.info = av.Info{Key: "live/room"}

	good := newFakeWriter("good")
	bad := newFakeWriter("bad")
	bad.fail = true
	s.AddWriter(good)
	s.AddWriter(bad)

	r := newFakeReader("pub")
	s.AddReader(r)
	r.packets <- video(t, 1, 0x17, 0x00, 0, 0, 0)
	r.packets <- video(t, 2, 0x17, 0x01, 0, 0, 0)

	_, ok := s.GetWs().Load("bad")
	assert.False(t, ok, "failing writer is removed")
	_, ok = s.GetWs().Load("good")
	assert.True(t, ok)

	close(r.packets)
	waitClosed(t, good.closed)
	assert.Equal(t, []uint32{1, 2}, good.timestamps())
}

func TestStreamCopyMovesWriters(t *testing.T) {
	cfg := DefaultConfig()
	old := NewStream(&cfg)
	old.info = av.Info{Key: "live/room"}
	old.AddWriter(newFakeWriter("a"))
	old.AddWriter(newFakeWriter("b"))

	ns := NewStream(&cfg)
	old.Copy(ns)

	assert.Equal(t, old.info, ns.info)
	for _, uid := range []string{"a", "b"} {
		_, inOld := old.GetWs().Load(uid)
		_, inNew := ns.GetWs().Load(uid)
		assert.False(t, inOld, uid)
		assert.True(t, inNew, uid)
	}
}

func TestStreamIDWithoutReader(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, EmptyID, NewStream(&cfg).ID())
}

func TestCheckAppName(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.CheckAppName("live"))
	assert.False(t, cfg.CheckAppName("other"))
	assert.False(t, cfg.CheckAppName(""))
}
