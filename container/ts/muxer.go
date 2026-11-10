package ts

import (
	"encoding/binary"
	"io"

	"github.com/ClipMyHorseTV/livego/av"
)

const (
	tsDefaultDataLen = 184
	tsPacketLen      = 188
	h264DefaultHZ    = 90

	videoPID = 0x100
	audioPID = 0x101
	videoSID = 0xe0
	audioSID = 0xc0
)

type Muxer struct {
	videoCc  byte
	audioCc  byte
	patCc    byte
	pmtCc    byte
	pat      [tsPacketLen]byte
	pmt      [tsPacketLen]byte
	tsPacket [tsPacketLen]byte
}

func NewMuxer() *Muxer {
	return &Muxer{}
}

func (muxer *Muxer) Mux(p *av.Packet, w io.Writer) error {
	first := true
	wBytes := 0
	pesIndex := 0

	var pes pesHeader
	dts := int64(p.TimeStamp) * int64(h264DefaultHZ)
	pts := dts
	pid := audioPID
	isKeyFrame := false
	if p.IsVideo {
		pid = videoPID
		videoH, _ := p.Header.(av.VideoPacketHeader)
		pts = dts + int64(videoH.CompositionTime())*int64(h264DefaultHZ)
		isKeyFrame = videoH.IsKeyFrame()
	}
	err := pes.packet(p, pts, dts)
	if err != nil {
		return err
	}
	pesHeaderLen := pes.len
	packetBytesLen := len(p.Data) + int(pesHeaderLen)

	for packetBytesLen > 0 {
		i := muxer.writeTSHeader(pid, muxer.nextCC(p.IsVideo), first)

		// 关键帧需要加pcr
		if first && isKeyFrame {
			i = muxer.writePCRField(i, dts)
		}

		// frame data
		var dataLen byte
		dataLen, i = muxer.reservePayload(i, first, packetBytesLen)

		var tmpLen byte
		if first && i < tsPacketLen && pesHeaderLen > 0 {
			tmpLen = min(pesHeaderLen, tsPacketLen-i)
			copy(muxer.tsPacket[i:], pes.data[pesIndex:pesIndex+int(tmpLen)])
			i += tmpLen
			packetBytesLen -= int(tmpLen)
			dataLen -= tmpLen
			pesHeaderLen -= tmpLen
			pesIndex += int(tmpLen)
		}

		if i < tsPacketLen {
			tmpLen = tsPacketLen - i
			if tmpLen <= dataLen {
				dataLen = tmpLen
			}
			copy(muxer.tsPacket[i:], p.Data[wBytes:wBytes+int(dataLen)])
			wBytes += int(dataLen)
			packetBytesLen -= int(dataLen)
		}
		if w != nil {
			if _, err := w.Write(muxer.tsPacket[0:]); err != nil {
				return err
			}
		}
		first = false
	}

	return nil
}

// reservePayload determines the payload length of the current TS packet, whose header ends at i.
// When the remaining data does not fill the packet, it pads the packet with an adaptation field.
// It returns the payload length and the position where the payload starts.
func (muxer *Muxer) reservePayload(i byte, first bool, packetBytesLen int) (dataLen, pos byte) {
	if packetBytesLen <= 0 {
		return 0, i
	}
	if packetBytesLen >= tsDefaultDataLen {
		dataLen = tsDefaultDataLen
		if first {
			dataLen -= (i - 4)
		}
		return dataLen, i
	}

	muxer.tsPacket[3] |= 0x20 // have adaptation
	dataLen = byte(packetBytesLen)
	var remainBytes byte
	if first {
		remainBytes = tsDefaultDataLen - dataLen - (i - 4)
	} else {
		remainBytes = tsDefaultDataLen - dataLen
	}
	muxer.adaptationBufInit(muxer.tsPacket[i:], remainBytes)
	return dataLen, i + remainBytes
}

// nextCC advances and returns the continuity counter of the audio or video stream.
func (muxer *Muxer) nextCC(isVideo bool) byte {
	if isVideo {
		muxer.videoCc = (muxer.videoCc + 1) & 0x0f
		return muxer.videoCc
	}
	muxer.audioCc = (muxer.audioCc + 1) & 0x0f
	return muxer.audioCc
}

// writeTSHeader writes the 4-byte TS packet header and returns the write position after it.
func (muxer *Muxer) writeTSHeader(pid int, cc byte, first bool) byte {
	i := byte(0)

	// sync byte
	muxer.tsPacket[i] = 0x47
	i++

	// error indicator, unit start indicator,ts priority,pid
	muxer.tsPacket[i] = byte((pid >> 8) & 0xff) // pid high 5 bits
	if first {
		muxer.tsPacket[i] |= 0x40 // unit start indicator
	}
	i++

	// pid low 8 bits
	muxer.tsPacket[i] = byte(pid & 0xff)
	i++

	// scram control, adaptation control, counter
	muxer.tsPacket[i] = 0x10 | cc&0x0f
	i++

	return i
}

// PAT return pat data.
func (muxer *Muxer) PAT() []byte {
	i := 0
	tsHeader := []byte{0x47, 0x40, 0x00, 0x10, 0x00}
	patHeader := []byte{0x00, 0xb0, 0x0d, 0x00, 0x01, 0xc1, 0x00, 0x00, 0x00, 0x01, 0xf0, 0x01}

	if muxer.patCc > 0xf {
		muxer.patCc = 0
	}
	tsHeader[3] |= muxer.patCc & 0x0f
	muxer.patCc++

	copy(muxer.pat[i:], tsHeader)
	i += len(tsHeader)

	copy(muxer.pat[i:], patHeader)
	i += len(patHeader)

	binary.BigEndian.PutUint32(muxer.pat[i:], GenCrc32(patHeader))
	i += 4

	remainByte := tsPacketLen - i
	for j := range remainByte {
		muxer.pat[i+j] = 0xff
	}

	return muxer.pat[0:]
}

// PMT return pmt data.
func (muxer *Muxer) PMT(soundFormat byte, hasVideo bool) []byte {
	var progInfo []byte
	tsHeader := []byte{0x47, 0x50, 0x01, 0x10, 0x00}
	pmtHeader := []byte{0x02, 0xb0, 0xff, 0x00, 0x01, 0xc1, 0x00, 0x00, 0xe1, 0x00, 0xf0, 0x00}
	if !hasVideo {
		pmtHeader[9] = 0x01
		progInfo = []byte{0x0f, 0xe1, 0x01, 0xf0, 0x00}
	} else {
		progInfo = []byte{
			0x1b, 0xe1, 0x00, 0xf0, 0x00, // h264 or h265*
			0x0f, 0xe1, 0x01, 0xf0, 0x00, // mp3 or aac
		}
	}
	pmtHeader[2] = byte((len(progInfo) + 9 + 4) & 0xff) // section length, low 8 bits

	if muxer.pmtCc > 0xf {
		muxer.pmtCc = 0
	}
	tsHeader[3] |= muxer.pmtCc & 0x0f
	muxer.pmtCc++

	if soundFormat == 2 ||
		soundFormat == 14 {
		if hasVideo {
			progInfo[5] = 0x4
		} else {
			progInfo[0] = 0x4
		}
	}

	i := 0
	copy(muxer.pmt[i:], tsHeader)
	i += len(tsHeader)

	copy(muxer.pmt[i:], pmtHeader)
	i += len(pmtHeader)

	copy(muxer.pmt[i:], progInfo[0:])
	i += len(progInfo)

	binary.BigEndian.PutUint32(muxer.pmt[i:], GenCrc32(muxer.pmt[5:5+len(pmtHeader)+len(progInfo)]))
	i += 4

	remainBytes := tsPacketLen - i
	for j := range remainBytes {
		muxer.pmt[i+j] = 0xff
	}

	return muxer.pmt[0:]
}

func (muxer *Muxer) adaptationBufInit(src []byte, remainBytes byte) {
	src[0] = remainBytes - 1
	if remainBytes == 1 {
		return
	}
	src[1] = 0x00
	for i := 2; i < len(src); i++ {
		src[i] = 0xff
	}
}

// writePCRField writes an adaptation field carrying the PCR at position i and returns the
// write position after it.
func (muxer *Muxer) writePCRField(i byte, dts int64) byte {
	muxer.tsPacket[3] |= 0x20
	muxer.tsPacket[i] = 7
	i++
	muxer.tsPacket[i] = 0x50
	i++
	muxer.writePcr(muxer.tsPacket[0:], i, dts)
	return i + 6
}

func (muxer *Muxer) writePcr(b []byte, i byte, pcr int64) {
	b[i] = byte((pcr >> 25) & 0xff)
	i++
	b[i] = byte((pcr >> 17) & 0xff)
	i++
	b[i] = byte((pcr >> 9) & 0xff)
	i++
	b[i] = byte((pcr >> 1) & 0xff)
	i++
	b[i] = byte(pcr&0x1)<<7 | 0x7e
	i++
	b[i] = 0x00
}

type pesHeader struct {
	len  byte
	data [tsPacketLen]byte
}

// packet return pes packet.
func (header *pesHeader) packet(p *av.Packet, pts, dts int64) error {
	// PES header
	i := 0
	header.data[i] = 0x00
	i++
	header.data[i] = 0x00
	i++
	header.data[i] = 0x01
	i++

	sid := audioSID
	if p.IsVideo {
		sid = videoSID
	}
	header.data[i] = byte(sid)
	i++

	flag := 0x80
	ptslen := 5
	dtslen := ptslen
	headerSize := ptslen
	if p.IsVideo && pts != dts {
		flag |= 0x40
		headerSize += 5 // add dts
	}
	size := len(p.Data) + headerSize + 3
	if size > 0xffff {
		size = 0
	}
	header.data[i] = byte(size >> 8)
	i++
	header.data[i] = byte(size)
	i++

	header.data[i] = 0x80
	i++
	header.data[i] = byte(flag)
	i++
	header.data[i] = byte(headerSize)
	i++

	header.writeTs(header.data[0:], i, flag>>6, pts)
	i += ptslen
	if p.IsVideo && pts != dts {
		header.writeTs(header.data[0:], i, 1, dts)
		i += dtslen
	}

	header.len = byte(i)

	return nil
}

func (header *pesHeader) writeTs(src []byte, i int, fb int, ts int64) {
	if ts > 0x1ffffffff {
		ts -= 0x1ffffffff
	}
	val := int64(fb<<4) | ((ts>>30)&0x07)<<1 | 1
	src[i] = byte(val & 0xff)
	i++

	val = ((ts>>15)&0x7fff)<<1 | 1
	src[i] = byte((val >> 8) & 0xff)
	i++
	src[i] = byte(val & 0xff)
	i++

	val = (ts&0x7fff)<<1 | 1
	src[i] = byte((val >> 8) & 0xff)
	i++
	src[i] = byte(val & 0xff)
}
