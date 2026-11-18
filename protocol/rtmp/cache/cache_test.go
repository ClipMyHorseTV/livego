package cache

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/container/flv"
)

type recorder struct {
	packets []*av.Packet
	failAt  int // fail the n-th write (1-based), 0 = never
}

func (r *recorder) Write(p *av.Packet) error {
	if r.failAt > 0 && len(r.packets)+1 == r.failAt {
		return errors.New("write failed")
	}
	r.packets = append(r.packets, p)
	return nil
}
func (r *recorder) Info() av.Info      { return av.Info{} }
func (r *recorder) Close(error)        {}
func (r *recorder) Alive() bool        { return true }
func (r *recorder) CalcBaseTimestamp() {}

// timestamps returns the timestamps of the recorded packets, which the tests use as packet IDs.
func (r *recorder) timestamps() []uint32 {
	ts := make([]uint32, 0, len(r.packets))
	for _, p := range r.packets {
		ts = append(ts, p.TimeStamp)
	}
	return ts
}

func packet(t *testing.T, ts uint32, isVideo bool, data ...byte) *av.Packet {
	t.Helper()
	p := &av.Packet{IsVideo: isVideo, IsAudio: !isVideo, TimeStamp: ts, Data: data}
	require.NoError(t, flv.NewDemuxer().DemuxH(p))
	return p
}

func videoSeq(t *testing.T, ts uint32) *av.Packet {
	return packet(t, ts, true, 0x17, 0x00, 0, 0, 0)
}

func keyFrame(t *testing.T, ts uint32) *av.Packet {
	return packet(t, ts, true, 0x17, 0x01, 0, 0, 0)
}

func interFrame(t *testing.T, ts uint32) *av.Packet {
	return packet(t, ts, true, 0x27, 0x01, 0, 0, 0)
}

func aacSeq(t *testing.T, ts uint32) *av.Packet {
	return packet(t, ts, false, 0xaf, 0x00, 0x12, 0x10)
}

func aacRaw(t *testing.T, ts uint32) *av.Packet {
	return packet(t, ts, false, 0xaf, 0x01, 0x21)
}

func TestGopCacheIgnoresPacketsBeforeFirstKeyFrame(t *testing.T) {
	g := NewGopCache(1)
	g.Write(interFrame(t, 1))
	g.Write(interFrame(t, 2))
	g.Write(keyFrame(t, 3))
	g.Write(interFrame(t, 4))

	var r recorder
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{3, 4}, r.timestamps())
}

func TestGopCacheEmpty(t *testing.T) {
	var r recorder
	require.NoError(t, NewGopCache(1).Send(&r))
	assert.Empty(t, r.packets)
}

func TestGopCacheKeepsOnlyLatestGop(t *testing.T) {
	g := NewGopCache(1)
	for _, p := range []*av.Packet{keyFrame(t, 1), interFrame(t, 2), keyFrame(t, 3), interFrame(t, 4), interFrame(t, 5)} {
		g.Write(p)
	}

	var r recorder
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{3, 4, 5}, r.timestamps())
}

func TestGopCacheTwoGops(t *testing.T) {
	g := NewGopCache(2)
	g.Write(keyFrame(t, 1))
	g.Write(interFrame(t, 2))

	var r recorder
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{1, 2}, r.timestamps(), "a partially filled ring is sent")

	g.Write(keyFrame(t, 3))
	g.Write(interFrame(t, 4))
	r = recorder{}
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{1, 2, 3, 4}, r.timestamps())

	g.Write(keyFrame(t, 5))
	g.Write(interFrame(t, 6))
	r = recorder{}
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{3, 4, 5, 6}, r.timestamps(), "oldest GOP is evicted, order is oldest first")
}

// Regression: with three or more GOPs, inter frames were written to an empty slot (nil pointer panic)
// and Send read the ring from the wrong position.
func TestGopCacheThreeOrMoreGops(t *testing.T) {
	for _, num := range []int{3, 4, 5} {
		t.Run(fmt.Sprintf("GopNum=%d", num), func(t *testing.T) {
			g := NewGopCache(num)
			var want []uint32
			for gop := range 2*num + 1 {
				key := uint32(gop * 10)
				g.Write(keyFrame(t, key))
				g.Write(interFrame(t, key+1))
				g.Write(interFrame(t, key+2))
				want = append(want, key, key+1, key+2)

				// The cache holds the last min(gop+1, num) GOPs, oldest first.
				if len(want) > 3*num {
					want = want[3:]
				}
				var r recorder
				require.NoError(t, g.Send(&r))
				assert.Equal(t, want, r.timestamps(), "after %d GOPs", gop+1)
			}
		})
	}
}

func TestCacheWithConfiguredGopNum(t *testing.T) {
	c := NewCache(Config{GopNum: 3})
	c.Write(*videoSeq(t, 1))
	for _, ts := range []uint32{10, 20, 30, 40} {
		c.Write(*keyFrame(t, ts))
		c.Write(*interFrame(t, ts+1))
	}

	var r recorder
	require.NoError(t, c.Send(&r))
	assert.Equal(t, []uint32{1, 20, 21, 30, 31, 40, 41}, r.timestamps())
}

func TestGopCacheTruncatesLongGop(t *testing.T) {
	g := NewGopCache(1)
	g.Write(keyFrame(t, 0))
	for i := range maxGOPCap + 100 {
		g.Write(interFrame(t, uint32(i+1)))
	}

	var r recorder
	require.NoError(t, g.Send(&r))
	assert.Len(t, r.packets, maxGOPCap)

	// The next key frame starts a fresh GOP.
	g.Write(keyFrame(t, 99999))
	r = recorder{}
	require.NoError(t, g.Send(&r))
	assert.Equal(t, []uint32{99999}, r.timestamps())
}

func TestGopCacheSendStopsOnWriteError(t *testing.T) {
	g := NewGopCache(1)
	g.Write(keyFrame(t, 1))
	g.Write(interFrame(t, 2))
	g.Write(interFrame(t, 3))

	r := recorder{failAt: 2}
	require.Error(t, g.Send(&r))
	assert.Equal(t, []uint32{1}, r.timestamps())
}

func TestCacheSendOrder(t *testing.T) {
	c := NewCache(DefaultConfig())

	// Written in an order different from the one a new player needs.
	c.Write(*keyFrame(t, 10))
	c.Write(*aacSeq(t, 2))
	c.Write(*interFrame(t, 11))
	c.Write(*videoSeq(t, 1))
	c.Write(av.Packet{IsMetadata: true, TimeStamp: 0})

	var r recorder
	require.NoError(t, c.Send(&r))
	// metadata, video sequence header, audio sequence header, then the GOP
	assert.Equal(t, []uint32{0, 1, 2, 10, 11}, r.timestamps())
}

func TestCacheKeepsLatestSequenceHeaders(t *testing.T) {
	c := NewCache(DefaultConfig())
	c.Write(*videoSeq(t, 1))
	c.Write(*videoSeq(t, 2))
	c.Write(*aacSeq(t, 3))
	c.Write(*aacSeq(t, 4))

	var r recorder
	require.NoError(t, c.Send(&r))
	assert.Equal(t, []uint32{2, 4}, r.timestamps())
}

func TestCacheDoesNotCacheRawAudio(t *testing.T) {
	c := NewCache(DefaultConfig())
	c.Write(*keyFrame(t, 1))
	c.Write(*aacRaw(t, 2))

	var r recorder
	require.NoError(t, c.Send(&r))
	assert.Equal(t, []uint32{1}, r.timestamps())
}

func TestCacheSendPropagatesError(t *testing.T) {
	c := NewCache(DefaultConfig())
	c.Write(av.Packet{IsMetadata: true})
	c.Write(*videoSeq(t, 1))

	r := recorder{failAt: 1}
	require.Error(t, c.Send(&r))
	assert.Empty(t, r.packets)
}

func TestSpecialCacheSendsCopy(t *testing.T) {
	s := NewSpecialCache()

	var r recorder
	require.NoError(t, s.Send(&r), "empty cache sends nothing")
	assert.Empty(t, r.packets)

	s.Write(&av.Packet{TimeStamp: 7, Data: []byte{1, 2, 3}})
	require.NoError(t, s.Send(&r))
	require.Len(t, r.packets, 1)

	// Consumers like the HLS demuxer reslice p.Data, which must not affect the cache.
	r.packets[0].Data = r.packets[0].Data[2:]

	r = recorder{}
	require.NoError(t, s.Send(&r))
	assert.Equal(t, []byte{1, 2, 3}, r.packets[0].Data)
}
