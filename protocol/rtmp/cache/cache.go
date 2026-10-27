package cache

import (
	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/configure"
)

type Cache struct {
	gop      *GopCache
	videoSeq *SpecialCache
	audioSeq *SpecialCache
	metadata *SpecialCache
}

func NewCache() *Cache {
	return &Cache{
		gop:      NewGopCache(configure.Config.GetInt("gop_num")),
		videoSeq: NewSpecialCache(),
		audioSeq: NewSpecialCache(),
		metadata: NewSpecialCache(),
	}
}

func (cache *Cache) Write(p av.Packet) {
	switch {
	case p.IsMetadata:
		cache.metadata.Write(&p)
	case p.IsVideo:
		vh, ok := p.Header.(av.VideoPacketHeader)
		if !ok {
			return
		}
		if vh.IsSeq() {
			cache.videoSeq.Write(&p)
			return
		}
		cache.gop.Write(&p)
	default:
		ah, ok := p.Header.(av.AudioPacketHeader)
		if !ok {
			cache.gop.Write(&p)
			return
		}
		// only the AAC sequence header is cached, other audio packets are not
		if ah.SoundFormat() == av.SoundAAC && ah.AACPacketType() == av.AACSeqHdr {
			cache.audioSeq.Write(&p)
		}
	}
}

func (cache *Cache) Send(w av.WriteCloser) error {
	if err := cache.metadata.Send(w); err != nil {
		return err
	}

	if err := cache.videoSeq.Send(w); err != nil {
		return err
	}

	if err := cache.audioSeq.Send(w); err != nil {
		return err
	}

	if err := cache.gop.Send(w); err != nil {
		return err
	}

	return nil
}
