package rtmp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/ClipMyHorseTV/livego/utils/uid"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/configure"
	"github.com/ClipMyHorseTV/livego/container/flv"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"

	log "github.com/sirupsen/logrus"
)

const (
	maxQueueNum         = 1024
	SaveStaticsInterval = 5000
)

var (
	readTimeout  = configure.Config.GetInt("read_timeout")
	writeTimeout = configure.Config.GetInt("write_timeout")
)

type Client struct {
	handler av.Handler
	getter  av.GetWriter
}

func NewRtmpClient(h av.Handler, getter av.GetWriter) *Client {
	return &Client{
		handler: h,
		getter:  getter,
	}
}

func (c *Client) Dial(url string, method string) error {
	connClient := core.NewConnClient()
	if err := connClient.Start(url, method); err != nil {
		return err
	}
	switch method {
	case av.PUBLISH:
		writer := NewVirWriter(connClient)
		log.Debugf("client Dial call NewVirWriter url=%s, method=%s", url, method)
		c.handler.HandleWriter(writer)
	case av.PLAY:
		reader := NewVirReader(connClient)
		log.Debugf("client Dial call NewVirReader url=%s, method=%s", url, method)
		c.handler.HandleReader(reader)
		if c.getter != nil {
			writer := c.getter.GetWriter(reader.Info())
			c.handler.HandleWriter(writer)
		}
	}
	return nil
}

func (c *Client) GetHandle() av.Handler {
	return c.handler
}

type Server struct {
	handler av.Handler
	getter  av.GetWriter
}

func NewRtmpServer(h av.Handler, getter av.GetWriter) *Server {
	return &Server{
		handler: h,
		getter:  getter,
	}
}

func (s *Server) Serve(listener net.Listener) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("rtmp serve panic: ", r)
		}
	}()

	for {
		var netconn net.Conn
		netconn, err = listener.Accept()
		if err != nil {
			return err
		}
		conn := core.NewConn(netconn, 4*1024)
		log.Debug("new client, connect remote: ", conn.RemoteAddr().String(),
			"local:", conn.LocalAddr().String())
		// handleConn logs its own errors
		go func() { _ = s.handleConn(conn) }()
	}
}

// closeConn closes conn after a failed setup, logging the cause.
func closeConn(conn *core.Conn, msg string, cause error) {
	log.Error(msg, cause)
	if err := conn.Close(); err != nil {
		log.Debug("close rtmp conn: ", err)
	}
}

func (s *Server) handleConn(conn *core.Conn) error {
	if err := conn.HandshakeServer(); err != nil {
		closeConn(conn, "handleConn HandshakeServer err: ", err)
		return err
	}
	connServer := core.NewConnServer(conn)

	if err := connServer.ReadMsg(); err != nil {
		closeConn(conn, "handleConn read msg err: ", err)
		return err
	}

	appname, name, _ := connServer.GetInfo()

	if ret := configure.CheckAppName(appname); !ret {
		err := fmt.Errorf("application name=%s is not configured", appname)
		closeConn(conn, "CheckAppName err: ", err)
		return err
	}

	log.Debugf("handleConn: IsPublisher=%v", connServer.IsPublisher())
	if !connServer.IsPublisher() {
		writer := NewVirWriter(connServer)
		log.Debugf("new player: %+v", writer.Info())
		s.handler.HandleWriter(writer)
		return nil
	}

	return s.handlePublisher(conn, connServer, appname, name)
}

// handlePublisher authorizes a publishing connection by its stream key and attaches its reader
// and the configured writers to the handler.
func (s *Server) handlePublisher(conn *core.Conn, connServer *core.ConnServer, appname, name string) error {
	if configure.Config.GetBool("rtmp_noauth") {
		key, err := configure.RoomKeys.GetKey(name)
		if err != nil {
			err = fmt.Errorf("cannot create key: %w", err)
			closeConn(conn, "GetKey err: ", err)
			return err
		}
		name = key
	}
	channel, err := configure.RoomKeys.GetChannel(name)
	if err != nil {
		err = fmt.Errorf("invalid key: %w", err)
		closeConn(conn, "CheckKey err: ", err)
		return err
	}
	connServer.PublishInfo.Name = channel
	if pushlist, ret := configure.GetStaticPushURLList(appname); ret && (pushlist != nil) {
		log.Debugf("GetStaticPushUrlList: %v", pushlist)
	}
	reader := NewVirReader(connServer)
	s.handler.HandleReader(reader)
	log.Debugf("new publisher: %+v", reader.Info())

	if s.getter != nil {
		writeType := reflect.TypeOf(s.getter)
		log.Debugf("handleConn:writeType=%v", writeType)
		writer := s.getter.GetWriter(reader.Info())
		s.handler.HandleWriter(writer)
	}
	if configure.Config.GetBool("flv_archive") {
		flvWriter := new(flv.FlvDvr)
		s.handler.HandleWriter(flvWriter.GetWriter(reader.Info()))
	}
	return nil
}

type GetInFo interface {
	GetInfo() (string, string, string)
}

type StreamReadWriteCloser interface {
	GetInFo
	Close(error)
	Write(core.ChunkStream) error
	Read(c *core.ChunkStream) error
}

type StaticsBW struct {
	StreamID               uint32
	VideoDatainBytes       uint64
	LastVideoDatainBytes   uint64
	VideoSpeedInBytesperMS uint64

	AudioDatainBytes       uint64
	LastAudioDatainBytes   uint64
	AudioSpeedInBytesperMS uint64

	LastTimestamp int64
}

// update adds length bytes of audio or video to the totals and recomputes the speeds once
// SaveStaticsInterval has passed since the last computation.
func (bw *StaticsBW) update(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := time.Now().UnixMilli()

	bw.StreamID = streamid
	if isVideoFlag {
		bw.VideoDatainBytes += length
	} else {
		bw.AudioDatainBytes += length
	}

	if bw.LastTimestamp == 0 {
		bw.LastTimestamp = nowInMS
		return
	}
	elapsedMS := nowInMS - bw.LastTimestamp
	if elapsedMS < SaveStaticsInterval {
		return
	}
	diffTimestamp := uint64(elapsedMS / 1000)

	bw.VideoSpeedInBytesperMS = (bw.VideoDatainBytes - bw.LastVideoDatainBytes) * 8 / diffTimestamp / 1000
	bw.AudioSpeedInBytesperMS = (bw.AudioDatainBytes - bw.LastAudioDatainBytes) * 8 / diffTimestamp / 1000

	bw.LastVideoDatainBytes = bw.VideoDatainBytes
	bw.LastAudioDatainBytes = bw.AudioDatainBytes
	bw.LastTimestamp = nowInMS
}

// streamKey returns the stream key ("app/name") contained in a stream URL.
func streamKey(streamURL string) string {
	u, err := url.Parse(streamURL)
	if err != nil {
		log.Warning(err)
		return ""
	}
	return strings.TrimLeft(u.Path, "/")
}

type VirWriter struct {
	av.RWBaser

	Uid         string
	closed      bool
	conn        StreamReadWriteCloser
	packetQueue chan *av.Packet
	WriteBWInfo StaticsBW
}

func NewVirWriter(conn StreamReadWriteCloser) *VirWriter {
	ret := &VirWriter{
		Uid:         uid.NewID(),
		conn:        conn,
		RWBaser:     av.NewRWBaser(time.Second * time.Duration(writeTimeout)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}

	go ret.Check()
	go func() {
		// SendPacket only returns once the writer fails or is closed
		log.Warning(ret.SendPacket())
	}()
	return ret
}

func (v *VirWriter) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	v.WriteBWInfo.update(streamid, length, isVideoFlag)
}

func (v *VirWriter) Check() {
	var c core.ChunkStream
	for {
		if err := v.conn.Read(&c); err != nil {
			v.Close(err)
			return
		}
	}
}

func (v *VirWriter) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info)
	for range maxQueueNum - 84 {
		tmpPkt, ok := <-pktQue
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			if len(pktQue) > maxQueueNum-2 {
				log.Debug("drop audio pkt")
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
				log.Debug("drop video pkt")
				<-pktQue
			}
		}
	}
	log.Debug("packet queue len: ", len(pktQue))
}

func (v *VirWriter) Write(p *av.Packet) (err error) {
	err = nil

	if v.closed {
		err = errors.New("VirWriter closed")
		return err
	}
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("VirWriter has already been closed:%v", e)
		}
	}()
	if len(v.packetQueue) >= maxQueueNum-24 {
		v.DropPacket(v.packetQueue, v.Info())
	} else {
		v.packetQueue <- p
	}

	return err
}

// SendPacket writes queued packets to the connection until the queue is closed or a write fails.
// It never returns a nil error.
func (v *VirWriter) SendPacket() error {
	flusher, canFlush := v.conn.(interface{ Flush() error })
	for {
		p, ok := <-v.packetQueue
		if !ok {
			return errors.New("closed")
		}
		cs, err := v.chunkFromPacket(p)
		if err != nil {
			v.closed = true
			return err
		}

		v.SaveStatics(p.StreamID, uint64(cs.Length), p.IsVideo)
		v.SetPreTime()
		v.RecTimeStamp(cs.Timestamp, cs.TypeID)
		if err := v.conn.Write(cs); err != nil {
			v.closed = true
			return err
		}
		if canFlush {
			if err := flusher.Flush(); err != nil {
				v.closed = true
				return err
			}
		}
	}
}

// chunkFromPacket builds the RTMP message for p.
func (v *VirWriter) chunkFromPacket(p *av.Packet) (core.ChunkStream, error) {
	var cs core.ChunkStream
	length, err := core.MessageLength(len(p.Data))
	if err != nil {
		return cs, err
	}
	cs.Data = p.Data
	cs.Length = length
	cs.StreamID = p.StreamID
	cs.Timestamp = p.TimeStamp
	cs.Timestamp += v.BaseTimeStamp()

	switch {
	case p.IsVideo:
		cs.TypeID = av.TagVideo
	case p.IsMetadata:
		cs.TypeID = av.TagScriptDataAMF0
	default:
		cs.TypeID = av.TagAudio
	}
	return cs, nil
}

func (v *VirWriter) Info() (ret av.Info) {
	ret.UID = v.Uid
	_, _, u := v.conn.GetInfo()
	ret.URL = u
	ret.Key = streamKey(u)
	ret.Inter = true
	return ret
}

func (v *VirWriter) Close(err error) {
	log.Warning("player ", v.Info(), "closed: "+err.Error())
	if !v.closed {
		close(v.packetQueue)
	}
	v.closed = true
	v.conn.Close(err)
}

type VirReader struct {
	av.RWBaser

	Uid        string
	demuxer    *flv.Demuxer
	conn       StreamReadWriteCloser
	ReadBWInfo StaticsBW
}

func NewVirReader(conn StreamReadWriteCloser) *VirReader {
	return &VirReader{
		Uid:        uid.NewID(),
		conn:       conn,
		RWBaser:    av.NewRWBaser(time.Second * time.Duration(readTimeout)),
		demuxer:    flv.NewDemuxer(),
		ReadBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}
}

func (v *VirReader) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	v.ReadBWInfo.update(streamid, length, isVideoFlag)
}

func (v *VirReader) Read(p *av.Packet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Warning("rtmp read packet panic: ", r)
		}
	}()

	v.SetPreTime()
	var cs core.ChunkStream
	for {
		err = v.conn.Read(&cs)
		if err != nil {
			return err
		}
		if cs.TypeID == av.TagAudio ||
			cs.TypeID == av.TagVideo ||
			cs.TypeID == av.TagScriptDataAMF0 ||
			cs.TypeID == av.TagScriptDataAMF3 {
			break
		}
	}

	p.IsAudio = cs.TypeID == av.TagAudio
	p.IsVideo = cs.TypeID == av.TagVideo
	p.IsMetadata = cs.TypeID == av.TagScriptDataAMF0 || cs.TypeID == av.TagScriptDataAMF3
	p.StreamID = cs.StreamID
	p.Data = cs.Data
	p.TimeStamp = cs.Timestamp

	v.SaveStatics(p.StreamID, uint64(len(p.Data)), p.IsVideo)
	// a packet with an unparsable header is still forwarded; consumers check p.Header
	if err := v.demuxer.DemuxH(p); err != nil {
		log.Debug("rtmp demux packet header: ", err)
	}
	return nil
}

func (v *VirReader) Info() (ret av.Info) {
	ret.UID = v.Uid
	_, _, u := v.conn.GetInfo()
	ret.URL = u
	ret.Key = streamKey(u)
	return ret
}

func (v *VirReader) Close(err error) {
	log.Debug("publisher ", v.Info(), "closed: "+err.Error())
	v.conn.Close(err)
}
