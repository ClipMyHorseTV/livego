package av

import (
	"fmt"
	"io"
)

const (
	TagAudio          = 8
	TagVideo          = 9
	TagScriptDataAMF0 = 18
	TagScriptDataAMF3 = 0xf
)

const (
	MetadatAMF0  = 0x12
	MetadataAMF3 = 0xf
)

const (
	SoundMP3                 = 2
	SoundNellymoser16kHzMono = 4
	SoundNellymoser8kHzMono  = 5
	SoundNellymoser          = 6
	SoundALaw                = 7
	SoundMuLaw               = 8
	SoundAAC                 = 10
	SoundSpeex               = 11

	Sound5p5kHz = 0
	Sound11kHz  = 1
	Sound22kHz  = 2
	Sound44kHz  = 3

	Sound8Bit  = 0
	Sound16Bit = 1

	SoundMono   = 0
	SoundStereo = 1

	AACSeqHdr = 0
	AACRaw    = 1
)

const (
	AVCSeqHdr = 0
	AVCNALU   = 1
	AVCEOS    = 2

	FrameKey   = 1
	FrameInter = 2

	VideoH264 = 7
)

var (
	PUBLISH = "publish"
	PLAY    = "play"
)

// Packet is a single audio, video or metadata packet.
// Header can be converted to AudioHeaderInfo or VideoHeaderInfo.
type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	TimeStamp  uint32 // dts
	StreamID   uint32
	Header     PacketHeader
	Data       []byte
}

type PacketHeader any

type AudioPacketHeader interface {
	PacketHeader
	SoundFormat() uint8
	AACPacketType() uint8
}

type VideoPacketHeader interface {
	PacketHeader
	IsKeyFrame() bool
	IsSeq() bool
	CodecID() uint8
	CompositionTime() int32
}

type Demuxer interface {
	Demux(*Packet) (ret *Packet, err error)
}

type Muxer interface {
	Mux(*Packet, io.Writer) error
}

type SampleRater interface {
	SampleRate() (int, error)
}

type CodecParser interface {
	SampleRater
	Parse(*Packet, io.Writer) error
}

type GetWriter interface {
	GetWriter(Info) WriteCloser
}

type Handler interface {
	HandleReader(ReadCloser)
	HandleWriter(WriteCloser)
}

type Alive interface {
	Alive() bool
}

type Closer interface {
	Info() Info
	Close(error)
}

type CalcTime interface {
	CalcBaseTimestamp()
}

type Info struct {
	Key   string
	URL   string
	UID   string
	Inter bool
}

func (info Info) IsInterval() bool {
	return info.Inter
}

func (info Info) String() string {
	return fmt.Sprintf("<key: %s, URL: %s, UID: %s, Inter: %v>",
		info.Key, info.URL, info.UID, info.Inter)
}

type ReadCloser interface {
	Closer
	Alive
	Read(*Packet) error
}

type WriteCloser interface {
	Closer
	Alive
	CalcTime
	Write(*Packet) error
}
