package rtmprelay

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/configure"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"

	log "github.com/sirupsen/logrus"
)

type StaticPush struct {
	RtmpURL       string
	packetChan    chan *av.Packet
	sndctrlChan   chan string
	connectClient *core.ConnClient
	startflag     bool
}

var (
	StaticPushMap          = make(map[string](*StaticPush))
	mapLock                = new(sync.RWMutex)
	PushURLList   []string = nil
)

var StaticRelayStopCtrl = "STATIC_RTMPRELAY_STOP"

func GetStaticPushList(appname string) ([]string, error) {
	if PushURLList == nil {
		// Do not unmarshel the config every time, lots of reflect works -gs
		pushurlList, ok := configure.GetStaticPushURLList(appname)
		if !ok {
			PushURLList = []string{}
		} else {
			PushURLList = pushurlList
		}
	}

	if len(PushURLList) == 0 {
		return nil, errors.New("no static push url")
	}

	return PushURLList, nil
}

func GetAndCreateStaticPushObject(rtmpurl string) *StaticPush {
	mapLock.RLock()
	staticpush, ok := StaticPushMap[rtmpurl]
	log.Debugf("GetAndCreateStaticPushObject: %s, return %v", rtmpurl, ok)
	if !ok {
		mapLock.RUnlock()
		newStaticpush := NewStaticPush(rtmpurl)

		mapLock.Lock()
		StaticPushMap[rtmpurl] = newStaticpush
		mapLock.Unlock()

		return newStaticpush
	}
	mapLock.RUnlock()

	return staticpush
}

func GetStaticPushObject(rtmpurl string) (*StaticPush, error) {
	mapLock.RLock()
	if staticpush, ok := StaticPushMap[rtmpurl]; ok {
		mapLock.RUnlock()
		return staticpush, nil
	}
	mapLock.RUnlock()

	return nil, fmt.Errorf("StaticPushMap[%s] does not exist", rtmpurl)
}

func ReleaseStaticPushObject(rtmpurl string) {
	mapLock.RLock()
	if _, ok := StaticPushMap[rtmpurl]; ok {
		mapLock.RUnlock()

		log.Debugf("ReleaseStaticPushObject %s ok", rtmpurl)
		mapLock.Lock()
		delete(StaticPushMap, rtmpurl)
		mapLock.Unlock()
	} else {
		mapLock.RUnlock()
		log.Debugf("ReleaseStaticPushObject: not find %s", rtmpurl)
	}
}

func NewStaticPush(rtmpurl string) *StaticPush {
	return &StaticPush{
		RtmpURL:       rtmpurl,
		packetChan:    make(chan *av.Packet, 500),
		sndctrlChan:   make(chan string),
		connectClient: nil,
		startflag:     false,
	}
}

func (sp *StaticPush) Start() error {
	if sp.startflag {
		return fmt.Errorf("StaticPush already start %s", sp.RtmpURL)
	}

	sp.connectClient = core.NewConnClient()

	log.Debugf("static publish server addr:%v starting....", sp.RtmpURL)
	err := sp.connectClient.Start(sp.RtmpURL, "publish")
	if err != nil {
		log.Debugf("connectClient.Start url=%v error", sp.RtmpURL)
		return err
	}
	log.Debugf("static publish server addr:%v started, streamid=%d", sp.RtmpURL, sp.connectClient.GetStreamID())
	go sp.HandleAvPacket()

	sp.startflag = true
	return nil
}

func (sp *StaticPush) Stop() {
	if !sp.startflag {
		return
	}

	log.Debugf("StaticPush Stop: %s", sp.RtmpURL)
	sp.sndctrlChan <- StaticRelayStopCtrl
	sp.startflag = false
}

func (sp *StaticPush) WriteAvPacket(packet *av.Packet) {
	if !sp.startflag {
		return
	}

	sp.packetChan <- packet
}

func (sp *StaticPush) sendPacket(p *av.Packet) {
	if !sp.startflag {
		return
	}
	var cs core.ChunkStream

	length, err := core.MessageLength(len(p.Data))
	if err != nil {
		log.Debugf("static push %s: %v", sp.RtmpURL, err)
		return
	}
	cs.Data = p.Data
	cs.Length = length
	cs.StreamID = sp.connectClient.GetStreamID()
	cs.Timestamp = p.TimeStamp
	// cs.Timestamp += v.BaseTimeStamp()

	// log.Printf("Static sendPacket: rtmpurl=%s, length=%d, streamid=%d",
	//	self.RtmpUrl, len(p.Data), cs.StreamID)
	if p.IsVideo {
		cs.TypeID = av.TagVideo
	} else {
		if p.IsMetadata {
			cs.TypeID = av.TagScriptDataAMF0
		} else {
			cs.TypeID = av.TagAudio
		}
	}

	if err := sp.connectClient.Write(cs); err != nil {
		log.Debugf("static push %s write: %v", sp.RtmpURL, err)
	}
}

func (sp *StaticPush) HandleAvPacket() {
	if !sp.IsStart() {
		log.Debugf("static push %s not started", sp.RtmpURL)
		return
	}

	for {
		select {
		case packet := <-sp.packetChan:
			sp.sendPacket(packet)
		case ctrlcmd := <-sp.sndctrlChan:
			if ctrlcmd == StaticRelayStopCtrl {
				sp.connectClient.Close(nil)
				log.Debugf("Static HandleAvPacket close: publishurl=%s", sp.RtmpURL)
				return
			}
		}
	}
}

func (sp *StaticPush) IsStart() bool {
	return sp.startflag
}
