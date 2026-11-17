package parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/container/flv"
)

// demuxed builds a packet the way the HLS source sees it: header parsed, tag header stripped.
func demuxed(t *testing.T, isVideo bool, data []byte) *av.Packet {
	t.Helper()
	p := &av.Packet{IsVideo: isVideo, IsAudio: !isVideo, Data: data}
	require.NoError(t, flv.NewDemuxer().Demux(p))
	return p
}

func TestSampleRateWithoutAudio(t *testing.T) {
	_, err := NewCodecParser().SampleRate()
	require.ErrorIs(t, err, errNoAudio)
}

func TestParseAACDispatch(t *testing.T) {
	cp := NewCodecParser()
	var buf bytes.Buffer

	// 48 kHz AAC-LC sequence header
	require.NoError(t, cp.Parse(demuxed(t, false, []byte{0xaf, 0x00, 0x11, 0x90}), &buf))
	rate, err := cp.SampleRate()
	require.NoError(t, err)
	assert.Equal(t, 48000, rate)
	assert.Zero(t, buf.Len(), "sequence header produces no output")

	require.NoError(t, cp.Parse(demuxed(t, false, []byte{0xaf, 0x01, 0x21, 0x22}), &buf))
	assert.Equal(t, 7+2, buf.Len(), "raw frame is written with an ADTS header")
}

func TestParseMP3Dispatch(t *testing.T) {
	cp := NewCodecParser()
	require.NoError(t, cp.Parse(demuxed(t, false, []byte{0x2f, 0xff, 0xfb, 0x94}), nil))
	rate, err := cp.SampleRate()
	require.NoError(t, err)
	assert.Equal(t, 48000, rate)
}

func TestParseIgnoresPacketsWithoutHeader(t *testing.T) {
	cp := NewCodecParser()
	require.NoError(t, cp.Parse(&av.Packet{IsVideo: true, Data: []byte{1}}, nil))
	require.NoError(t, cp.Parse(&av.Packet{IsAudio: true, Data: []byte{1}}, nil))
	_, err := cp.SampleRate()
	require.ErrorIs(t, err, errNoAudio)
}
