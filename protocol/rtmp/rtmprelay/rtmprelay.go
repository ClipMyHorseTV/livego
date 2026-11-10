package rtmprelay

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/protocol/amf"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"

	log "github.com/sirupsen/logrus"
)

var StopCtrl = "RTMPRELAY_STOP"

type RtmpRelay struct {
	PlayURL              string
	PublishURL           string
	csChan               chan core.ChunkStream
	sndctrlChan          chan string
	connectPlayClient    *core.ConnClient
	connectPublishClient *core.ConnClient
	startflag            bool
}

func NewRtmpRelay(playurl *string, publishurl *string) *RtmpRelay {
	return &RtmpRelay{
		PlayURL:              *playurl,
		PublishURL:           *publishurl,
		csChan:               make(chan core.ChunkStream, 500),
		sndctrlChan:          make(chan string),
		connectPlayClient:    nil,
		connectPublishClient: nil,
		startflag:            false,
	}
}

func (r *RtmpRelay) rcvPlayChunkStream() {
	log.Debug("rcvPlayRtmpMediaPacket connectClient.Read...")
	for {
		var rc core.ChunkStream

		if !r.startflag {
			r.connectPlayClient.Close(nil)
			log.Debugf("rcvPlayChunkStream close: playurl=%s, publishurl=%s", r.PlayURL, r.PublishURL)
			break
		}
		err := r.connectPlayClient.Read(&rc)

		if err != nil && errors.Is(err, io.EOF) {
			break
		}
		// log.Debugf("connectPlayClient.Read return rc.TypeID=%v length=%d, err=%v", rc.TypeID, len(rc.Data), err)
		switch rc.TypeID {
		case 20, 17:
			reader := bytes.NewReader(rc.Data)
			vs, err := r.connectPlayClient.DecodeBatch(reader, amf.AMF0)

			log.Debugf("rcvPlayRtmpMediaPacket: vs=%v, err=%v", vs, err)
		case 18:
			log.Debug("rcvPlayRtmpMediaPacket: metadata....")
			r.csChan <- rc
		case 8, 9:
			r.csChan <- rc
		}
	}
}

func (r *RtmpRelay) sendPublishChunkStream() {
	for {
		select {
		case rc := <-r.csChan:
			// log.Debugf("sendPublishChunkStream: rc.TypeID=%v length=%d", rc.TypeID, len(rc.Data))
			if err := r.connectPublishClient.Write(rc); err != nil {
				log.Debugf("sendPublishChunkStream write: %v", err)
			}
		case ctrlcmd := <-r.sndctrlChan:
			if ctrlcmd == StopCtrl {
				r.connectPublishClient.Close(nil)
				log.Debugf("sendPublishChunkStream close: playurl=%s, publishurl=%s", r.PlayURL, r.PublishURL)
				return
			}
		}
	}
}

func (r *RtmpRelay) Start() error {
	if r.startflag {
		return fmt.Errorf("the rtmprelay already started, playurl=%s, publishurl=%s", r.PlayURL, r.PublishURL)
	}

	r.connectPlayClient = core.NewConnClient()
	r.connectPublishClient = core.NewConnClient()

	log.Debugf("play server addr:%v starting....", r.PlayURL)
	err := r.connectPlayClient.Start(r.PlayURL, av.PLAY)
	if err != nil {
		log.Debugf("connectPlayClient.Start url=%v error", r.PlayURL)
		return err
	}

	log.Debugf("publish server addr:%v starting....", r.PublishURL)
	err = r.connectPublishClient.Start(r.PublishURL, av.PUBLISH)
	if err != nil {
		log.Debugf("connectPublishClient.Start url=%v error", r.PublishURL)
		r.connectPlayClient.Close(nil)
		return err
	}

	r.startflag = true
	go r.rcvPlayChunkStream()
	go r.sendPublishChunkStream()

	return nil
}

func (r *RtmpRelay) Stop() {
	if !r.startflag {
		log.Debugf("The rtmprelay already stoped, playurl=%s, publishurl=%s", r.PlayURL, r.PublishURL)
		return
	}

	r.startflag = false
	r.sndctrlChan <- StopCtrl
}
