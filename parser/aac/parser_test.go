package aac

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
)

// AudioSpecificConfig for AAC-LC, 44.1 kHz, stereo.
var lc44100Stereo = []byte{0x12, 0x10}

func TestParseSequenceHeader(t *testing.T) {
	p := NewParser()
	require.NoError(t, p.Parse(lc44100Stereo, av.AACSeqHdr, nil))

	assert.Equal(t, byte(2), p.cfgInfo.objectType)
	assert.Equal(t, byte(4), p.cfgInfo.sampleRate)
	assert.Equal(t, byte(2), p.cfgInfo.channel)
	assert.Equal(t, 44100, p.SampleRate())
}

func TestParseSequenceHeaderTooShort(t *testing.T) {
	require.ErrorIs(t, NewParser().Parse([]byte{0x12}, av.AACSeqHdr, nil), errSpecificBufInvalid)
}

func TestRawBeforeSequenceHeader(t *testing.T) {
	var buf bytes.Buffer
	err := NewParser().Parse([]byte{0x01, 0x02}, av.AACRaw, &buf)
	require.ErrorIs(t, err, errAudioBufInvalid)
	assert.Zero(t, buf.Len())
}

func TestADTSHeader(t *testing.T) {
	p := NewParser()
	require.NoError(t, p.Parse(lc44100Stereo, av.AACSeqHdr, nil))

	payload := bytes.Repeat([]byte{0xab}, 100)
	var buf bytes.Buffer
	require.NoError(t, p.Parse(payload, av.AACRaw, &buf))

	out := buf.Bytes()
	require.Len(t, out, adtsHeaderLen+len(payload))
	h := out[:adtsHeaderLen]

	// Well known header for AAC-LC 44.1 kHz stereo with a 107 byte frame.
	assert.Equal(t, []byte{0xff, 0xf1, 0x50, 0x80, 0x0d, 0x7f, 0xfc}, h)

	// Decode the fields to make the expectation readable.
	assert.Equal(t, 0xfff, int(h[0])<<4|int(h[1])>>4, "syncword")
	assert.Equal(t, 1, int(h[2]>>6), "profile (object type - 1)")
	assert.Equal(t, 4, int(h[2]>>2)&0x0f, "sampling frequency index")
	assert.Equal(t, 2, int(h[2]&0x01)<<2|int(h[3]>>6), "channel configuration")
	frameLen := int(h[3]&0x03)<<11 | int(h[4])<<3 | int(h[5])>>5
	assert.Equal(t, adtsHeaderLen+len(payload), frameLen, "frame length includes the header")

	assert.Equal(t, payload, out[adtsHeaderLen:])
}

func TestADTSFrameLengthLimit(t *testing.T) {
	p := NewParser()
	require.NoError(t, p.Parse(lc44100Stereo, av.AACSeqHdr, nil))

	var buf bytes.Buffer
	require.NoError(t, p.Parse(make([]byte, maxADTSFrameLen-adtsHeaderLen), av.AACRaw, &buf))

	buf.Reset()
	err := p.Parse(make([]byte, maxADTSFrameLen-adtsHeaderLen+1), av.AACRaw, &buf)
	require.ErrorIs(t, err, errAudioBufTooLarge)
	assert.Zero(t, buf.Len(), "nothing must be written for a rejected frame")
}

func TestSampleRate(t *testing.T) {
	tests := []struct {
		name string
		cfg  []byte
		want int
	}{
		{name: "48 kHz", cfg: []byte{0x11, 0x90}, want: 48000},
		{name: "22.05 kHz", cfg: []byte{0x13, 0x90}, want: 22050},
		{name: "invalid index falls back to 44.1 kHz", cfg: []byte{0x17, 0x90}, want: 44100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			require.NoError(t, p.Parse(tt.cfg, av.AACSeqHdr, nil))
			assert.Equal(t, tt.want, p.SampleRate())
		})
	}
}
