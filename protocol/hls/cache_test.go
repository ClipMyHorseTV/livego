package hls

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTSCacheItemEvictsOldest(t *testing.T) {
	c := NewTSCacheItem("live/room")
	assert.Equal(t, "live/room", c.ID())

	for i := range maxTSCacheNum + 1 {
		name := fmt.Sprintf("/live/room/%d.ts", i)
		c.SetItem(name, NewTSItem(name, 2000, i, []byte{byte(i)}))
	}

	_, err := c.GetItem("/live/room/0.ts")
	require.ErrorIs(t, err, ErrNoKey)

	for i := 1; i <= maxTSCacheNum; i++ {
		item, err := c.GetItem(fmt.Sprintf("/live/room/%d.ts", i))
		require.NoError(t, err)
		assert.Equal(t, i, item.SeqNum)
		assert.Equal(t, []byte{byte(i)}, item.Data)
	}
}

func TestGenM3U8PlayList(t *testing.T) {
	c := NewTSCacheItem("live/room")
	c.SetItem("/a.ts", NewTSItem("/a.ts", 4000, 7, nil))
	c.SetItem("/b.ts", NewTSItem("/b.ts", 5500, 8, nil))
	c.SetItem("/c.ts", NewTSItem("/c.ts", 3250, 9, nil))

	got, err := c.GenM3U8PlayList()
	require.NoError(t, err)

	want := "#EXTM3U\n" +
		"#EXT-X-VERSION:3\n" +
		"#EXT-X-ALLOW-CACHE:NO\n" +
		"#EXT-X-TARGETDURATION:6\n" + // longest segment (5.5s) rounded up
		"#EXT-X-MEDIA-SEQUENCE:7\n" + // sequence number of the first listed segment
		"\n" +
		"#EXTINF:4.000,\n/a.ts\n" +
		"#EXTINF:5.500,\n/b.ts\n" +
		"#EXTINF:3.250,\n/c.ts\n"
	assert.Equal(t, want, string(got))
}

func TestGenM3U8PlayListMediaSequenceAdvances(t *testing.T) {
	c := NewTSCacheItem("live/room")
	for i := range maxTSCacheNum + 2 {
		name := fmt.Sprintf("/%d.ts", i)
		c.SetItem(name, NewTSItem(name, 1000, i, nil))
	}

	got, err := c.GenM3U8PlayList()
	require.NoError(t, err)
	assert.Contains(t, string(got), "#EXT-X-MEDIA-SEQUENCE:2\n")
	assert.NotContains(t, string(got), "/0.ts")
	assert.NotContains(t, string(got), "/1.ts")
}

func TestNewTSItemCopiesData(t *testing.T) {
	data := []byte{1, 2, 3}
	item := NewTSItem("/a.ts", 1000, 1, data)
	data[0] = 0xff
	assert.Equal(t, []byte{1, 2, 3}, item.Data)
}

func TestAlign(t *testing.T) {
	const inc = 3600 // 40ms at 90kHz
	var a align

	// The first frame sets the base.
	dts := uint64(90000)
	a.align(&dts, inc)
	assert.Equal(t, uint64(90000), dts)

	// Small jitter (below 2ms) snaps to the estimated timestamp.
	dts = 90000 + inc + 90
	a.align(&dts, inc)
	assert.Equal(t, uint64(90000+inc), dts)

	dts = 90000 + 2*inc - 90
	a.align(&dts, inc)
	assert.Equal(t, uint64(90000+2*inc), dts)

	// A larger gap resets the base and keeps the original timestamp.
	dts = 90000 + 10*inc
	a.align(&dts, inc)
	assert.Equal(t, uint64(90000+10*inc), dts)

	dts = 90000 + 11*inc + 10
	a.align(&dts, inc)
	assert.Equal(t, uint64(90000+11*inc), dts)
}

func TestStatusDuration(t *testing.T) {
	s := newStatus()
	s.update(false, 1000)
	s.update(true, 1500)
	s.update(false, 3200)
	assert.True(t, s.hasVideo)
	assert.Equal(t, int64(2200), s.durationMs())

	s.resetAndNew()
	assert.Equal(t, int64(1), s.seqID)
	assert.False(t, s.hasVideo)
	s.update(false, 5000)
	assert.Equal(t, int64(0), s.durationMs(), "first timestamp is reset for the new segment")
}
