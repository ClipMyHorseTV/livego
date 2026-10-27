package aac

import (
	"errors"
	"io"

	"github.com/ClipMyHorseTV/livego/av"
)

type mpegExtension struct {
	objectType byte
	sampleRate byte
}

type mpegCfgInfo struct {
	objectType     byte
	sampleRate     byte
	channel        byte
	sbr            byte
	ps             byte
	frameLen       byte
	exceptionLogTs int64
	extension      *mpegExtension
}

var aacRates = []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

var (
	errSpecificBufInvalid = errors.New("audio mpegspecific error")
	errAudioBufInvalid    = errors.New("audiodata  invalid")
	errAudioBufTooLarge   = errors.New("audiodata too large for an ADTS frame")
)

const (
	adtsHeaderLen   = 7
	maxADTSFrameLen = 0x1fff
)

type Parser struct {
	gettedSpecific bool
	adtsHeader     []byte
	cfgInfo        *mpegCfgInfo
}

func NewParser() *Parser {
	return &Parser{
		gettedSpecific: false,
		cfgInfo:        &mpegCfgInfo{},
		adtsHeader:     make([]byte, adtsHeaderLen),
	}
}

func (parser *Parser) specificInfo(src []byte) error {
	if len(src) < 2 {
		return errSpecificBufInvalid
	}
	parser.gettedSpecific = true
	parser.cfgInfo.objectType = (src[0] >> 3) & 0xff
	parser.cfgInfo.sampleRate = ((src[0] & 0x07) << 1) | src[1]>>7
	parser.cfgInfo.channel = (src[1] >> 3) & 0x0f
	return nil
}

func (parser *Parser) adts(src []byte, w io.Writer) error {
	if len(src) == 0 || !parser.gettedSpecific {
		return errAudioBufInvalid
	}

	// The ADTS frame_length field is 13 bits wide and includes the 7-byte header.
	n := len(src) + adtsHeaderLen
	if n > maxADTSFrameLen {
		return errAudioBufTooLarge
	}
	frameLen := uint16(n)

	// first write adts header
	parser.adtsHeader[0] = 0xff
	parser.adtsHeader[1] = 0xf1

	parser.adtsHeader[2] &= 0x00
	parser.adtsHeader[2] |= (parser.cfgInfo.objectType - 1) << 6
	parser.adtsHeader[2] |= parser.cfgInfo.sampleRate << 2

	parser.adtsHeader[3] &= 0x00
	parser.adtsHeader[3] |= (parser.cfgInfo.channel << 2) << 4
	parser.adtsHeader[3] |= byte((frameLen >> 11) & 0x03)

	parser.adtsHeader[4] &= 0x00
	parser.adtsHeader[4] |= byte((frameLen >> 3) & 0xff)

	parser.adtsHeader[5] &= 0x00
	parser.adtsHeader[5] |= byte(frameLen&0x07) << 5
	parser.adtsHeader[5] |= (0x7C << 1) >> 3
	parser.adtsHeader[6] = 0xfc

	if _, err := w.Write(parser.adtsHeader[0:]); err != nil {
		return err
	}
	if _, err := w.Write(src); err != nil {
		return err
	}
	return nil
}

func (parser *Parser) SampleRate() int {
	rate := 44100
	if int(parser.cfgInfo.sampleRate) < len(aacRates) {
		rate = aacRates[parser.cfgInfo.sampleRate]
	}
	return rate
}

func (parser *Parser) Parse(b []byte, packetType uint8, w io.Writer) (err error) {
	switch packetType {
	case av.AACSeqHdr:
		err = parser.specificInfo(b)
	case av.AACRaw:
		err = parser.adts(b, w)
	}
	return err
}
