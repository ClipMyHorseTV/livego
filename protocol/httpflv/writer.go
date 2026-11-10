package httpflv

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/protocol/amf"
	"github.com/ClipMyHorseTV/livego/utils/pio"
	"github.com/ClipMyHorseTV/livego/utils/uid"

	log "github.com/sirupsen/logrus"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
	// maxTagDataLen is the largest payload an FLV tag can describe (24-bit DataSize field).
	maxTagDataLen = 0xffffff
)

type FLVWriter struct {
	av.RWBaser

	Uid             string
	app, title, url string
	buf             []byte
	closed          bool
	closedChan      chan struct{}
	ctx             http.ResponseWriter
	packetQueue     chan *av.Packet
}

func NewFLVWriter(app, title, url string, ctx http.ResponseWriter) *FLVWriter {
	ret := &FLVWriter{
		Uid:         uid.NewID(),
		app:         app,
		title:       title,
		url:         url,
		ctx:         ctx,
		RWBaser:     av.NewRWBaser(time.Second * 10),
		closedChan:  make(chan struct{}),
		buf:         make([]byte, headerLen),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}

	if _, err := ret.ctx.Write([]byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}); err != nil {
		log.Errorf("Error on response writer")
		ret.closed = true
	}
	pio.PutI32BE(ret.buf[:4], 0)
	if _, err := ret.ctx.Write(ret.buf[:4]); err != nil {
		log.Errorf("Error on response writer")
		ret.closed = true
	}
	go func() {
		// SendPacket only returns once the writer fails or is closed
		err := ret.SendPacket()
		log.Debug("SendPacket error: ", err)
		ret.closed = true
	}()
	return ret
}

func (flvWriter *FLVWriter) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info)
	for range maxQueueNum - 84 {
		tmpPkt, ok := <-pktQue
		if ok && tmpPkt.IsVideo {
			videoPkt, isVideoHeader := tmpPkt.Header.(av.VideoPacketHeader)
			// dont't drop sps config and dont't drop key frame
			if isVideoHeader && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
				log.Debug("insert keyframe to queue")
				pktQue <- tmpPkt
			}

			if len(pktQue) > maxQueueNum-10 {
				<-pktQue
			}
			// drop other packet
			<-pktQue
		}
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			log.Debug("insert audio to queue")
			pktQue <- tmpPkt
		}
	}
	log.Debug("packet queue len: ", len(pktQue))
}

func (flvWriter *FLVWriter) Write(p *av.Packet) (err error) {
	err = nil
	if flvWriter.closed {
		err = errors.New("flvwrite source closed")
		return err
	}

	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("FLVWriter has already been closed:%v", e)
		}
	}()

	if len(flvWriter.packetQueue) >= maxQueueNum-24 {
		flvWriter.DropPacket(flvWriter.packetQueue, flvWriter.Info())
	} else {
		flvWriter.packetQueue <- p
	}

	return err
}

// SendPacket writes queued packets to the response until the queue is closed or a write fails.
// It never returns a nil error.
func (flvWriter *FLVWriter) SendPacket() error {
	for {
		p, ok := <-flvWriter.packetQueue
		if !ok {
			return errors.New("closed")
		}
		if err := flvWriter.writeTag(p); err != nil {
			return err
		}
	}
}

// writeTag writes p as a single FLV tag followed by its PreviousTagSize.
func (flvWriter *FLVWriter) writeTag(p *av.Packet) error {
	flvWriter.RWBaser.SetPreTime()
	h := flvWriter.buf[:headerLen]
	typeID := av.TagVideo
	if !p.IsVideo {
		if p.IsMetadata {
			var err error
			typeID = av.TagScriptDataAMF0
			p.Data, err = amf.MetaDataReform(p.Data, amf.DEL)
			if err != nil {
				return err
			}
		} else {
			typeID = av.TagAudio
		}
	}
	dataLen := len(p.Data)
	if dataLen > maxTagDataLen {
		return fmt.Errorf("flv tag data too large: %d bytes", dataLen)
	}
	timestamp := p.TimeStamp
	timestamp += flvWriter.BaseTimeStamp()
	flvWriter.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampbase := timestamp & 0xffffff
	timestampExt := timestamp >> 24 & 0xff

	pio.PutU8(h[0:1], uint8(typeID))
	pio.PutI24BE(h[1:4], int32(dataLen))
	pio.PutI24BE(h[4:7], int32(timestampbase))
	pio.PutU8(h[7:8], uint8(timestampExt))

	if _, err := flvWriter.ctx.Write(h); err != nil {
		return err
	}

	if _, err := flvWriter.ctx.Write(p.Data); err != nil {
		return err
	}

	pio.PutI32BE(h[:4], int32(preDataLen))
	_, err := flvWriter.ctx.Write(h[:4])
	return err
}

func (flvWriter *FLVWriter) Wait() {
	<-flvWriter.closedChan
}

func (flvWriter *FLVWriter) Close(error) {
	log.Debug("http flv closed")
	if !flvWriter.closed {
		close(flvWriter.packetQueue)
		close(flvWriter.closedChan)
	}
	flvWriter.closed = true
}

func (flvWriter *FLVWriter) Info() (ret av.Info) {
	ret.UID = flvWriter.Uid
	ret.URL = flvWriter.url
	ret.Key = flvWriter.app + "/" + flvWriter.title
	ret.Inter = true
	return ret
}
