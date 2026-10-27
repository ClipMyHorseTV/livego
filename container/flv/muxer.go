package flv

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/configure"
	"github.com/ClipMyHorseTV/livego/protocol/amf"
	"github.com/ClipMyHorseTV/livego/utils/pio"
	"github.com/ClipMyHorseTV/livego/utils/uid"

	log "github.com/sirupsen/logrus"
)

var flvHeader = []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}

/*
func NewFlv(handler av.Handler, info av.Info) {
	patths := strings.SplitN(info.Key, "/", 2)

	if len(patths) != 2 {
		log.Warning("invalid info")
		return
	}

	w, err := os.OpenFile(*flvFile, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Error("open file error: ", err)
	}

	writer := NewFLVWriter(patths[0], patths[1], info.URL, w)

	handler.HandleWriter(writer)

	writer.Wait()
	// close flv file
	log.Debug("close flv file")
	writer.ctx.Close()
}
*/

const (
	headerLen = 11
	// maxTagDataLen is the largest payload an FLV tag can describe (24-bit DataSize field).
	maxTagDataLen = 0xffffff
)

type FLVWriter struct {
	av.RWBaser

	Uid             string
	app, title, url string
	buf             []byte
	closed          chan struct{}
	ctx             *os.File
	closedWriter    bool
}

func NewFLVWriter(app, title, url string, ctx *os.File) (*FLVWriter, error) {
	ret := &FLVWriter{
		Uid:     uid.NewID(),
		app:     app,
		title:   title,
		url:     url,
		ctx:     ctx,
		RWBaser: av.NewRWBaser(time.Second * 10),
		closed:  make(chan struct{}),
		buf:     make([]byte, headerLen),
	}

	if _, err := ret.ctx.Write(flvHeader); err != nil {
		return nil, err
	}
	pio.PutI32BE(ret.buf[:4], 0)
	if _, err := ret.ctx.Write(ret.buf[:4]); err != nil {
		return nil, err
	}

	return ret, nil
}

func (writer *FLVWriter) Write(p *av.Packet) error {
	writer.RWBaser.SetPreTime()
	h := writer.buf[:headerLen]
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
	timestamp += writer.BaseTimeStamp()
	writer.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampbase := timestamp & 0xffffff
	timestampExt := timestamp >> 24 & 0xff

	pio.PutU8(h[0:1], uint8(typeID))
	pio.PutI24BE(h[1:4], int32(dataLen))
	pio.PutI24BE(h[4:7], int32(timestampbase))
	pio.PutU8(h[7:8], uint8(timestampExt))

	if _, err := writer.ctx.Write(h); err != nil {
		return err
	}

	if _, err := writer.ctx.Write(p.Data); err != nil {
		return err
	}

	pio.PutI32BE(h[:4], int32(preDataLen))
	if _, err := writer.ctx.Write(h[:4]); err != nil {
		return err
	}

	return nil
}

func (writer *FLVWriter) Wait() {
	<-writer.closed
}

func (writer *FLVWriter) Close(error) {
	if writer.closedWriter {
		return
	}
	writer.closedWriter = true
	if err := writer.ctx.Close(); err != nil {
		log.Warning("close flv file: ", err)
	}
	close(writer.closed)
}

func (writer *FLVWriter) Info() (ret av.Info) {
	ret.UID = writer.Uid
	ret.URL = writer.url
	ret.Key = writer.app + "/" + writer.title
	return ret
}

type FlvDvr struct{}

func (f *FlvDvr) GetWriter(info av.Info) av.WriteCloser {
	paths := strings.SplitN(info.Key, "/", 2)
	if len(paths) != 2 {
		log.Warning("invalid info")
		return nil
	}

	flvDir := configure.Config.GetString("flv_dir")

	err := os.MkdirAll(path.Join(flvDir, paths[0]), 0o750)
	if err != nil {
		log.Error("mkdir error: ", err)
		return nil
	}

	fileName := fmt.Sprintf("%s_%d.%s", path.Join(flvDir, info.Key), time.Now().Unix(), "flv")
	log.Debug("flv dvr save stream to: ", fileName)
	w, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Error("open file error: ", err)
		return nil
	}

	writer, err := NewFLVWriter(paths[0], paths[1], info.URL, w)
	if err != nil {
		log.Error("write flv header: ", err)
		_ = w.Close()
		return nil
	}
	log.Debug("new flv dvr: ", writer.Info())
	return writer
}
