package hls

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/ClipMyHorseTV/livego/configure"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/container/flv"
	"github.com/ClipMyHorseTV/livego/container/ts"
	"github.com/ClipMyHorseTV/livego/parser"

	log "github.com/sirupsen/logrus"
)

const (
	videoHZ      = 90000
	aacSampleLen = 1024
	maxQueueNum  = 512

	h264DefaultHz uint64 = 90
)

type Source struct {
	av.RWBaser

	seq         int
	info        av.Info
	bwriter     *bytes.Buffer
	btswriter   *bytes.Buffer
	demuxer     *flv.Demuxer
	muxer       *ts.Muxer
	pts, dts    uint64
	stat        *status
	align       *align
	cache       *audioCache
	tsCache     *TSCacheItem
	tsparser    *parser.CodecParser
	closed      bool
	packetQueue chan *av.Packet
}

func NewSource(info av.Info) *Source {
	info.Inter = true
	s := &Source{
		info:        info,
		align:       &align{},
		stat:        newStatus(),
		RWBaser:     av.NewRWBaser(time.Second * 10),
		cache:       newAudioCache(),
		demuxer:     flv.NewDemuxer(),
		muxer:       ts.NewMuxer(),
		tsCache:     NewTSCacheItem(info.Key),
		tsparser:    parser.NewCodecParser(),
		bwriter:     bytes.NewBuffer(make([]byte, 100*1024)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}
	go func() {
		err := s.SendPacket()
		if err != nil {
			log.Debug("send packet error: ", err)
			s.closed = true
		}
	}()
	return s
}

func (source *Source) GetCacheInc() *TSCacheItem {
	return source.tsCache
}

func (source *Source) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info)
	for range maxQueueNum - 84 {
		tmpPkt, ok := <-pktQue
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			if len(pktQue) > maxQueueNum-2 {
				<-pktQue
			} else {
				pktQue <- tmpPkt
			}
		}

		if ok && tmpPkt.IsVideo {
			videoPkt, ok := tmpPkt.Header.(av.VideoPacketHeader)
			// dont't drop sps config and dont't drop key frame
			if ok && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
				pktQue <- tmpPkt
			}
			if len(pktQue) > maxQueueNum-10 {
				<-pktQue
			}
		}
	}
	log.Debug("packet queue len: ", len(pktQue))
}

func (source *Source) Write(p *av.Packet) (err error) {
	err = nil
	if source.closed {
		err = errors.New("hls source closed")
		return err
	}
	source.SetPreTime()
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("hls source has already been closed:%v", e)
		}
	}()
	if len(source.packetQueue) >= maxQueueNum-24 {
		source.DropPacket(source.packetQueue, source.info)
	} else if !source.closed {
		source.packetQueue <- p
	}
	return err
}

func (source *Source) SendPacket() error {
	defer func() {
		log.Debugf("[%v] hls sender stop", source.info)
		if r := recover(); r != nil {
			log.Warning("hls SendPacket panic: ", r)
		}
	}()

	log.Debugf("[%v] hls sender start", source.info)
	for {
		if source.closed {
			return errors.New("closed")
		}

		p, ok := <-source.packetQueue
		if !ok {
			return errors.New("closed")
		}
		if err := source.sendPacket(p); err != nil {
			return err
		}
	}
}

// sendPacket demuxes and parses one packet and muxes it into the current TS segment.
// Only demuxer errors are returned; they stop the sender.
func (source *Source) sendPacket(p *av.Packet) error {
	if p.IsMetadata {
		return nil
	}

	err := source.demuxer.Demux(p)
	if errors.Is(err, flv.ErrAvcEndSEQ) {
		log.Warning(err)
		return nil
	}
	if err != nil {
		log.Warning(err)
		return err
	}

	compositionTime, isSeq, err := source.parse(p)
	if err != nil {
		log.Warning(err)
		return nil
	}
	if isSeq || source.btswriter == nil {
		return nil
	}

	source.stat.update(p.IsVideo, p.TimeStamp)
	source.calcPtsDts(p.IsVideo, p.TimeStamp, compositionTime)
	if err := source.tsMux(p); err != nil {
		log.Warning("hls: mux packet: ", err)
	}
	return nil
}

func (source *Source) Info() (ret av.Info) {
	return source.info
}

func (source *Source) cleanup() {
	close(source.packetQueue)
	source.bwriter = nil
	source.btswriter = nil
	source.cache = nil
	source.tsCache = nil
}

func (source *Source) Close(err error) {
	log.Debug("hls source closed: ", source.info)
	if !source.closed && !configure.Config.GetBool("hls_keep_after_end") {
		source.cleanup()
	}
	source.closed = true
}

func (source *Source) cut() {
	newf := true
	switch {
	case source.btswriter == nil:
		source.btswriter = bytes.NewBuffer(nil)
	case source.stat.durationMs() >= duration:
		if err := source.flushAudio(); err != nil {
			log.Warning("hls: flush audio: ", err)
		}

		source.seq++
		filename := fmt.Sprintf("/%s/%d.ts", source.info.Key, time.Now().Unix())
		item := NewTSItem(filename, int(source.stat.durationMs()), source.seq, source.btswriter.Bytes())
		source.tsCache.SetItem(filename, item)

		source.btswriter.Reset()
		source.stat.resetAndNew()
	default:
		newf = false
	}
	if newf {
		source.btswriter.Write(source.muxer.PAT())
		source.btswriter.Write(source.muxer.PMT(av.SoundAAC, true))
	}
}

func (source *Source) parse(p *av.Packet) (int32, bool, error) {
	var compositionTime int32
	var isSeq bool
	var err error
	if p.IsVideo {
		compositionTime, isSeq, err = videoHeaderInfo(p)
	} else {
		isSeq, err = audioHeaderInfo(p)
	}
	if err != nil {
		return compositionTime, false, err
	}
	if isSeq {
		return compositionTime, true, source.tsparser.Parse(p, source.bwriter)
	}
	source.bwriter.Reset()
	if err := source.tsparser.Parse(p, source.bwriter); err != nil {
		return compositionTime, false, err
	}
	p.Data = source.bwriter.Bytes()

	if p.IsVideo {
		if vh, ok := p.Header.(av.VideoPacketHeader); ok && vh.IsKeyFrame() {
			source.cut()
		}
	}
	return compositionTime, false, nil
}

// videoHeaderInfo checks that p is H.264 and returns its composition time and whether it is
// a sequence header.
func videoHeaderInfo(p *av.Packet) (int32, bool, error) {
	vh, ok := p.Header.(av.VideoPacketHeader)
	if !ok || vh.CodecID() != av.VideoH264 {
		return 0, false, ErrNoSupportVideoCodec
	}
	return vh.CompositionTime(), vh.IsKeyFrame() && vh.IsSeq(), nil
}

// audioHeaderInfo checks that p is AAC and returns whether it is a sequence header.
func audioHeaderInfo(p *av.Packet) (bool, error) {
	ah, ok := p.Header.(av.AudioPacketHeader)
	if !ok || ah.SoundFormat() != av.SoundAAC {
		return false, ErrNoSupportAudioCodec
	}
	return ah.AACPacketType() == av.AACSeqHdr, nil
}

func (source *Source) calcPtsDts(isVideo bool, ts uint32, compositionTs int32) {
	source.dts = uint64(ts) * h264DefaultHz
	source.pts = source.dts
	if isVideo {
		// a negative composition time offset is not supported and treated as 0
		if compositionTs > 0 {
			source.pts += uint64(compositionTs) * h264DefaultHz
		}
		return
	}

	sampleRate, err := source.tsparser.SampleRate()
	if err != nil || sampleRate <= 0 {
		return
	}
	frameDuration := uint64(videoHZ*aacSampleLen) / uint64(sampleRate)
	source.align.align(&source.dts, frameDuration)
	source.pts = source.dts
}

func (source *Source) flushAudio() error {
	return source.muxAudio(1)
}

func (source *Source) muxAudio(limit byte) error {
	if source.cache.CacheNum() < limit {
		return nil
	}
	var p av.Packet
	_, pts, buf := source.cache.GetFrame()
	p.Data = buf
	p.TimeStamp = uint32((pts / h264DefaultHz) & 0xffffffff) // FLV timestamps wrap at 32 bits
	return source.muxer.Mux(&p, source.btswriter)
}

func (source *Source) tsMux(p *av.Packet) error {
	if p.IsVideo {
		return source.muxer.Mux(p, source.btswriter)
	}
	source.cache.Cache(p.Data, source.pts)
	return source.muxAudio(cacheMaxFrames)
}
