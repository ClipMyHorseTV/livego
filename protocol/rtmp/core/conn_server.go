package core

import (
	"bytes"
	"errors"
	"io"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/protocol/amf"

	log "github.com/sirupsen/logrus"
)

var (
	publishLive   = "live"
	publishRecord = "record"
	publishAppend = "append"
)

var ErrReq = errors.New("req error")

var (
	cmdConnect       = "connect"
	cmdFcpublish     = "FCPublish"
	cmdReleaseStream = "releaseStream"
	cmdCreateStream  = "createStream"
	cmdPublish       = "publish"
	cmdFCUnpublish   = "FCUnpublish"
	cmdDeleteStream  = "deleteStream"
	cmdPlay          = "play"
)

const levelStatus = "status"

type ConnectInfo struct {
	App            string `amf:"app"            json:"app"`
	Flashver       string `amf:"flashVer"       json:"flashVer"`
	SwfURL         string `amf:"swfUrl"         json:"swfUrl"`
	TcURL          string `amf:"tcUrl"          json:"tcUrl"`
	Fpad           bool   `amf:"fpad"           json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs"    json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs"    json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction"  json:"videoFunction"`
	PageURL        string `amf:"pageUrl"        json:"pageUrl"`
	ObjectEncoding int    `amf:"objectEncoding" json:"objectEncoding"`
}

type ConnectResp struct {
	FMSVer       string `amf:"fmsVer"`
	Capabilities int    `amf:"capabilities"`
}

type ConnectEvent struct {
	Level          string `amf:"level"`
	Code           string `amf:"code"`
	Description    string `amf:"description"`
	ObjectEncoding int    `amf:"objectEncoding"`
}

type PublishInfo struct {
	Name string
	Type string
}

type ConnServer struct {
	done          bool
	streamID      int
	isPublisher   bool
	conn          *Conn
	transactionID int
	ConnInfo      ConnectInfo
	PublishInfo   PublishInfo
	decoder       *amf.Decoder
	encoder       *amf.Encoder
	bytesw        *bytes.Buffer
}

func NewConnServer(conn *Conn) *ConnServer {
	return &ConnServer{
		conn:     conn,
		streamID: 1,
		bytesw:   bytes.NewBuffer(nil),
		decoder:  &amf.Decoder{},
		encoder:  &amf.Encoder{},
	}
}

func (connServer *ConnServer) writeMsg(csid, streamID uint32, args ...any) error {
	connServer.bytesw.Reset()
	for _, v := range args {
		if _, err := connServer.encoder.Encode(connServer.bytesw, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := connServer.bytesw.Bytes()
	length, err := MessageLength(len(msg))
	if err != nil {
		return err
	}
	c := ChunkStream{
		Format:    0,
		CSID:      csid,
		Timestamp: 0,
		TypeID:    20,
		StreamID:  streamID,
		Length:    length,
		Data:      msg,
	}
	if err := connServer.conn.Write(&c); err != nil {
		return err
	}
	return connServer.conn.Flush()
}

func (connServer *ConnServer) connect(vs []any) error {
	for _, v := range vs {
		switch v := v.(type) {
		case float64:
			id := int(v)
			if id != 1 {
				return ErrReq
			}
			connServer.transactionID = id
		case amf.Object:
			if app, ok := v["app"].(string); ok {
				connServer.ConnInfo.App = app
			}
			if flashVer, ok := v["flashVer"].(string); ok {
				connServer.ConnInfo.Flashver = flashVer
			}
			if tcurl, ok := v["tcUrl"].(string); ok {
				connServer.ConnInfo.TcURL = tcurl
			}
			if encoding, ok := v["objectEncoding"].(float64); ok {
				connServer.ConnInfo.ObjectEncoding = int(encoding)
			}
		}
	}
	return nil
}

func (connServer *ConnServer) releaseStream(vs []any) error {
	return nil
}

func (connServer *ConnServer) fcPublish(vs []any) error {
	return nil
}

func (connServer *ConnServer) connectResp(cur *ChunkStream) error {
	for _, c := range []ChunkStream{
		connServer.conn.NewWindowAckSize(2500000),
		connServer.conn.NewSetPeerBandwidth(2500000),
		connServer.conn.NewSetChunkSize(uint32(1024)),
	} {
		if err := connServer.conn.Write(&c); err != nil {
			return err
		}
	}

	resp := make(amf.Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(amf.Object)
	event["level"] = levelStatus
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = connServer.ConnInfo.ObjectEncoding
	return connServer.writeMsg(cur.CSID, cur.StreamID, "_result", connServer.transactionID, resp, event)
}

func (connServer *ConnServer) createStream(vs []any) error {
	for _, v := range vs {
		switch v := v.(type) {
		case string:
		case float64:
			connServer.transactionID = int(v)
		case amf.Object:
		}
	}
	return nil
}

func (connServer *ConnServer) createStreamResp(cur *ChunkStream) error {
	return connServer.writeMsg(cur.CSID, cur.StreamID, "_result", connServer.transactionID, nil, connServer.streamID)
}

func (connServer *ConnServer) publishOrPlay(vs []any) error {
	for k, v := range vs {
		switch v := v.(type) {
		case string:
			switch k {
			case 2:
				connServer.PublishInfo.Name = v
			case 3:
				connServer.PublishInfo.Type = v
			}
		case float64:
			id := int(v)
			connServer.transactionID = id
		case amf.Object:
		}
	}

	return nil
}

func (connServer *ConnServer) publishResp(cur *ChunkStream) error {
	event := make(amf.Object)
	event["level"] = levelStatus
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publishing."
	return connServer.writeMsg(cur.CSID, cur.StreamID, "onStatus", 0, nil, event)
}

func (connServer *ConnServer) playResp(cur *ChunkStream) error {
	if err := connServer.conn.SetRecorded(); err != nil {
		return err
	}
	if err := connServer.conn.SetBegin(); err != nil {
		return err
	}

	event := make(amf.Object)
	event["level"] = levelStatus
	event["code"] = "NetStream.Play.Reset"
	event["description"] = "Playing and resetting stream."
	if err := connServer.writeMsg(cur.CSID, cur.StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = levelStatus
	event["code"] = "NetStream.Play.Start"
	event["description"] = "Started playing stream."
	if err := connServer.writeMsg(cur.CSID, cur.StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = levelStatus
	event["code"] = "NetStream.Data.Start"
	event["description"] = "Started playing stream."
	if err := connServer.writeMsg(cur.CSID, cur.StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = levelStatus
	event["code"] = "NetStream.Play.PublishNotify"
	event["description"] = "Started playing notify."
	if err := connServer.writeMsg(cur.CSID, cur.StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}
	return connServer.conn.Flush()
}

func (connServer *ConnServer) handleCmdMsg(c *ChunkStream) error {
	if c.TypeID == 17 {
		c.Data = c.Data[1:]
	}
	r := bytes.NewReader(c.Data)
	// DecodeBatch decodes until it fails, so io.EOF marks a completely decoded message.
	vs, err := connServer.decoder.DecodeBatch(r, amf.AMF0)
	if !errors.Is(err, io.EOF) {
		return err
	}
	// log.Debugf("rtmp req: %#v", vs)
	if len(vs) == 0 {
		return nil
	}
	cmd, ok := vs[0].(string)
	if !ok {
		return nil
	}
	return connServer.handleCmd(cmd, vs, c)
}

// handleCmd answers the command message cmd whose decoded values (including cmd itself) are vs.
func (connServer *ConnServer) handleCmd(cmd string, vs []any, c *ChunkStream) error {
	switch cmd {
	case cmdConnect:
		if err := connServer.connect(vs[1:]); err != nil {
			return err
		}
		return connServer.connectResp(c)
	case cmdCreateStream:
		if err := connServer.createStream(vs[1:]); err != nil {
			return err
		}
		return connServer.createStreamResp(c)
	case cmdPublish:
		if err := connServer.publishOrPlay(vs[1:]); err != nil {
			return err
		}
		if err := connServer.publishResp(c); err != nil {
			return err
		}
		connServer.done = true
		connServer.isPublisher = true
		log.Debug("handle publish req done")
	case cmdPlay:
		if err := connServer.publishOrPlay(vs[1:]); err != nil {
			return err
		}
		if err := connServer.playResp(c); err != nil {
			return err
		}
		connServer.done = true
		connServer.isPublisher = false
		log.Debug("handle play req done")
	case cmdFcpublish:
		return connServer.fcPublish(vs)
	case cmdReleaseStream:
		return connServer.releaseStream(vs)
	case cmdFCUnpublish, cmdDeleteStream:
	default:
		log.Debug("no support command=", cmd)
	}

	return nil
}

func (connServer *ConnServer) ReadMsg() error {
	var c ChunkStream
	for {
		if err := connServer.conn.Read(&c); err != nil {
			return err
		}
		switch c.TypeID {
		case 20, 17:
			if err := connServer.handleCmdMsg(&c); err != nil {
				return err
			}
		}
		if connServer.done {
			break
		}
	}
	return nil
}

func (connServer *ConnServer) IsPublisher() bool {
	return connServer.isPublisher
}

func (connServer *ConnServer) Write(c ChunkStream) error {
	if c.TypeID == av.TagScriptDataAMF0 ||
		c.TypeID == av.TagScriptDataAMF3 {
		var err error
		if c.Data, err = amf.MetaDataReform(c.Data, amf.DEL); err != nil {
			return err
		}
		if c.Length, err = MessageLength(len(c.Data)); err != nil {
			return err
		}
	}
	return connServer.conn.Write(&c)
}

func (connServer *ConnServer) Flush() error {
	return connServer.conn.Flush()
}

func (connServer *ConnServer) Read(c *ChunkStream) (err error) {
	return connServer.conn.Read(c)
}

func (connServer *ConnServer) GetInfo() (app string, name string, url string) {
	app = connServer.ConnInfo.App
	name = connServer.PublishInfo.Name
	url = connServer.ConnInfo.TcURL + "/" + connServer.PublishInfo.Name
	return app, name, url
}

func (connServer *ConnServer) Close(error) {
	if err := connServer.conn.Close(); err != nil {
		log.Debug("rtmp server conn close: ", err)
	}
}
