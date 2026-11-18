package flv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClipMyHorseTV/livego/av"
)

func TestParseAudioHeader(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		wantN         int
		wantFormat    uint8
		wantAACPacket uint8
		wantErr       bool
	}{
		{name: "aac sequence header", data: []byte{0xaf, 0x00, 0x12, 0x10}, wantN: 2, wantFormat: av.SoundAAC, wantAACPacket: av.AACSeqHdr},
		{name: "aac raw", data: []byte{0xaf, 0x01, 0x21}, wantN: 2, wantFormat: av.SoundAAC, wantAACPacket: av.AACRaw},
		{name: "mp3 has a one byte header", data: []byte{0x2f, 0xff, 0xfb}, wantN: 1, wantFormat: av.SoundMP3},
		{name: "empty", data: []byte{}, wantErr: true},
		{name: "aac without packet type", data: []byte{0xaf}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tag Tag
			n, err := tag.ParseMediaTagHeader(tt.data, false)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantN, n)
			assert.Equal(t, tt.wantFormat, tag.SoundFormat())
			assert.Equal(t, tt.wantAACPacket, tag.AACPacketType())
		})
	}
}

func TestParseAudioHeaderFlags(t *testing.T) {
	var tag Tag
	// 0xaf = AAC, 44kHz, 16 bit, stereo
	_, err := tag.ParseMediaTagHeader([]byte{0xaf, 0x01}, false)
	require.NoError(t, err)
	assert.Equal(t, uint8(av.Sound44kHz), tag.mediat.soundRate)
	assert.Equal(t, uint8(av.Sound16Bit), tag.mediat.soundSize)
	assert.Equal(t, uint8(av.SoundStereo), tag.mediat.soundType)
}

func TestParseVideoHeader(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantKey      bool
		wantSeq      bool
		wantCodec    uint8
		wantCompTime int32
		wantErr      bool
	}{
		{name: "avc sequence header", data: []byte{0x17, 0x00, 0x00, 0x00, 0x00}, wantKey: true, wantSeq: true, wantCodec: av.VideoH264},
		{name: "avc key frame", data: []byte{0x17, 0x01, 0x00, 0x00, 0x00, 0xaa}, wantKey: true, wantCodec: av.VideoH264},
		{name: "avc inter frame with composition time", data: []byte{0x27, 0x01, 0x00, 0x01, 0x02}, wantCodec: av.VideoH264, wantCompTime: 0x0102},
		{name: "negative composition time is sign extended", data: []byte{0x27, 0x01, 0xff, 0xff, 0xd8}, wantCodec: av.VideoH264, wantCompTime: -40},
		{name: "minimum composition time", data: []byte{0x27, 0x01, 0x80, 0x00, 0x00}, wantCodec: av.VideoH264, wantCompTime: -0x800000},
		{name: "maximum composition time", data: []byte{0x27, 0x01, 0x7f, 0xff, 0xff}, wantCodec: av.VideoH264, wantCompTime: 0x7fffff},
		{name: "too short", data: []byte{0x17, 0x01, 0x00, 0x00}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tag Tag
			n, err := tag.ParseMediaTagHeader(tt.data, true)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 5, n)
			assert.Equal(t, tt.wantKey, tag.IsKeyFrame())
			assert.Equal(t, tt.wantSeq, tag.IsSeq())
			assert.Equal(t, tt.wantCodec, tag.CodecID())
			assert.Equal(t, tt.wantCompTime, tag.CompositionTime())
		})
	}
}

func TestDemuxStripsTagHeader(t *testing.T) {
	d := NewDemuxer()

	video := &av.Packet{IsVideo: true, Data: []byte{0x17, 0x01, 0x00, 0x00, 0x00, 0xaa, 0xbb}}
	require.NoError(t, d.Demux(video))
	assert.Equal(t, []byte{0xaa, 0xbb}, video.Data)
	vh, ok := video.Header.(av.VideoPacketHeader)
	require.True(t, ok)
	assert.True(t, vh.IsKeyFrame())

	audio := &av.Packet{IsAudio: true, Data: []byte{0xaf, 0x01, 0xcc}}
	require.NoError(t, d.Demux(audio))
	assert.Equal(t, []byte{0xcc}, audio.Data)
	_, ok = audio.Header.(av.AudioPacketHeader)
	assert.True(t, ok)
}

func TestDemuxAVCEndOfSequence(t *testing.T) {
	p := &av.Packet{IsVideo: true, Data: []byte{0x17, 0x02, 0x00, 0x00, 0x00}}
	err := NewDemuxer().Demux(p)
	require.ErrorIs(t, err, ErrAvcEndSEQ)
}

func TestDemuxHKeepsPayload(t *testing.T) {
	data := []byte{0x27, 0x01, 0x00, 0x00, 0x00, 0xaa}
	p := &av.Packet{IsVideo: true, Data: data}
	require.NoError(t, NewDemuxer().DemuxH(p))
	assert.Equal(t, data, p.Data)
	assert.NotNil(t, p.Header)
}

func TestDemuxInvalidData(t *testing.T) {
	p := &av.Packet{IsVideo: true, Data: []byte{0x17}}
	require.Error(t, NewDemuxer().Demux(p))
}

func newTestFLVWriter(t *testing.T) (*FLVWriter, string) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "out.flv")
	f, err := os.Create(name)
	require.NoError(t, err)
	w, err := NewFLVWriter("live", "room", "rtmp://host/live/room", f)
	require.NoError(t, err)
	t.Cleanup(func() { w.Close(nil) })
	return w, name
}

func TestFLVWriterLayout(t *testing.T) {
	w, name := newTestFLVWriter(t)

	// The timestamp needs the extended byte: 0x01 goes to TimestampExtended.
	p := &av.Packet{IsVideo: true, TimeStamp: 0x01020304, Data: []byte{0xaa, 0xbb, 0xcc}}
	require.NoError(t, w.Write(p))
	w.Close(nil)

	got, err := os.ReadFile(name)
	require.NoError(t, err)

	want := []byte{
		// FLV header + PreviousTagSize0
		0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09,
		0x00, 0x00, 0x00, 0x00,
		// tag header: type, data size, timestamp, timestamp ext, stream id
		av.TagVideo, 0x00, 0x00, 0x03, 0x02, 0x03, 0x04, 0x01, 0x00, 0x00, 0x00,
		// payload
		0xaa, 0xbb, 0xcc,
		// PreviousTagSize = 11 + 3
		0x00, 0x00, 0x00, 0x0e,
	}
	assert.Equal(t, want, got)
}

func TestFLVWriterAudioTagAndBaseTimestamp(t *testing.T) {
	w, name := newTestFLVWriter(t)
	w.BaseTimestamp = 1000

	require.NoError(t, w.Write(&av.Packet{IsAudio: true, TimeStamp: 24, Data: []byte{0xaf, 0x01}}))
	w.Close(nil)

	got, err := os.ReadFile(name)
	require.NoError(t, err)
	tag := got[13:]
	assert.Equal(t, byte(av.TagAudio), tag[0])
	// 1024 = 0x000400
	assert.Equal(t, []byte{0x00, 0x04, 0x00}, tag[4:7])
	assert.Equal(t, uint32(1024), w.LastAudioTimestamp)
}

func TestFLVWriterRejectsOversizedTag(t *testing.T) {
	w, _ := newTestFLVWriter(t)
	err := w.Write(&av.Packet{IsVideo: true, Data: make([]byte, maxTagDataLen+1)})
	require.Error(t, err)
}

func TestFLVWriterCloseIsIdempotent(t *testing.T) {
	w, _ := newTestFLVWriter(t)
	w.Close(nil)
	assert.NotPanics(t, func() { w.Close(nil) })
	w.Wait() // returns because closed is closed
}

func TestFlvDvrGetWriter(t *testing.T) {
	dir := t.TempDir()
	dvr, err := NewFlvDvr(dir)
	require.NoError(t, err)

	assert.Nil(t, dvr.GetWriter(av.Info{Key: "no-slash"}))

	wc := dvr.GetWriter(av.Info{Key: "live/room", URL: "rtmp://x/live/room"})
	require.NotNil(t, wc)
	defer wc.Close(nil)
	assert.Equal(t, "live/room", wc.Info().Key)

	matches, err := filepath.Glob(filepath.Join(dir, "live", "room_*.flv"))
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}
