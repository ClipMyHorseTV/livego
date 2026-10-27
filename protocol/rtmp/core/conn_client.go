package core

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	neturl "net/url"
	"strings"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/configure"
	"github.com/ClipMyHorseTV/livego/protocol/amf"

	log "github.com/sirupsen/logrus"
)

var (
	respResult     = "_result"
	respError      = "_error"
	onStatus       = "onStatus"
	publishStart   = "NetStream.Publish.Start"
	playStart      = "NetStream.Play.Start"
	connectSuccess = "NetConnection.Connect.Success"
	onBWDone       = "onBWDone"
)

var ErrFail = errors.New("response err")

type ConnClient struct {
	done       bool
	transID    int
	url        string
	tcurl      string
	app        string
	title      string
	curcmdName string
	streamid   uint32
	isRTMPS    bool
	conn       *Conn
	encoder    *amf.Encoder
	decoder    *amf.Decoder
	bytesw     *bytes.Buffer
}

func NewConnClient() *ConnClient {
	return &ConnClient{
		transID: 1,
		bytesw:  bytes.NewBuffer(nil),
		encoder: &amf.Encoder{},
		decoder: &amf.Decoder{},
	}
}

func (connClient *ConnClient) DecodeBatch(r io.Reader, ver amf.Version) (ret []any, err error) {
	vs, err := connClient.decoder.DecodeBatch(r, ver)
	return vs, err
}

func (connClient *ConnClient) readRespMsg() error {
	var rc ChunkStream
	for {
		if err := connClient.conn.Read(&rc); err != nil {
			return err
		}
		if rc.TypeID != 20 && rc.TypeID != 17 {
			continue
		}

		r := bytes.NewReader(rc.Data)
		vs, _ := connClient.decoder.DecodeBatch(r, amf.AMF0)

		log.Debugf("readRespMsg: vs=%v", vs)
		for k, v := range vs {
			if err := connClient.checkRespValue(k, v); err != nil {
				return err
			}
		}

		return nil
	}
}

// checkRespValue validates the k-th value of a command response against the command that was sent.
func (connClient *ConnClient) checkRespValue(k int, v any) error {
	switch v := v.(type) {
	case string:
		return connClient.checkRespString(v)
	case float64:
		return connClient.checkRespNumber(k, v)
	case amf.Object:
		return connClient.checkRespObject(v)
	}
	return nil
}

func (connClient *ConnClient) checkRespString(v string) error {
	switch connClient.curcmdName {
	case cmdConnect, cmdCreateStream:
		if v != respResult {
			return errors.New(v)
		}
	case cmdPublish:
		if v != onStatus {
			return ErrFail
		}
	}
	return nil
}

func (connClient *ConnClient) checkRespNumber(k int, v float64) error {
	switch connClient.curcmdName {
	case cmdConnect, cmdCreateStream:
		switch k {
		case 1:
			if int(v) != connClient.transID {
				return ErrFail
			}
		case 3:
			if v < 0 || v > math.MaxUint32 {
				return fmt.Errorf("invalid stream id %v", v)
			}
			connClient.streamid = uint32(v)
		}
	case cmdPublish:
		if int(v) != 0 {
			return ErrFail
		}
	}
	return nil
}

func (connClient *ConnClient) checkRespObject(obj amf.Object) error {
	var want string
	switch connClient.curcmdName {
	case cmdConnect:
		want = connectSuccess
	case cmdPublish:
		want = publishStart
	default:
		return nil
	}
	code, ok := obj["code"]
	if !ok {
		return nil
	}
	if s, isString := code.(string); !isString || s != want {
		return ErrFail
	}
	return nil
}

func (connClient *ConnClient) writeMsg(args ...any) error {
	connClient.bytesw.Reset()
	for _, v := range args {
		if _, err := connClient.encoder.Encode(connClient.bytesw, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := connClient.bytesw.Bytes()
	length, err := MessageLength(len(msg))
	if err != nil {
		return err
	}
	c := ChunkStream{
		Format:    0,
		CSID:      3,
		Timestamp: 0,
		TypeID:    20,
		StreamID:  connClient.streamid,
		Length:    length,
		Data:      msg,
	}
	if err := connClient.conn.Write(&c); err != nil {
		return err
	}
	return connClient.conn.Flush()
}

func (connClient *ConnClient) writeConnectMsg() error {
	event := make(amf.Object)
	event["app"] = connClient.app
	event["type"] = "nonprivate"
	event["flashVer"] = "FMS.3.1"
	event["tcUrl"] = connClient.tcurl
	connClient.curcmdName = cmdConnect

	log.Debugf("writeConnectMsg: connClient.transID=%d, event=%v", connClient.transID, event)
	if err := connClient.writeMsg(cmdConnect, connClient.transID, event); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

func (connClient *ConnClient) writeCreateStreamMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdCreateStream

	log.Debugf("writeCreateStreamMsg: connClient.transID=%d", connClient.transID)
	if err := connClient.writeMsg(cmdCreateStream, connClient.transID, nil); err != nil {
		return err
	}

	for {
		err := connClient.readRespMsg()
		if err == nil {
			return nil
		}

		if errors.Is(err, ErrFail) {
			log.Debugf("writeCreateStreamMsg readRespMsg err=%v", err)
			return err
		}
	}
}

func (connClient *ConnClient) writePublishMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdPublish
	if err := connClient.writeMsg(cmdPublish, connClient.transID, nil, connClient.title, publishLive); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

func (connClient *ConnClient) writePlayMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdPlay
	log.Debugf("writePlayMsg: connClient.transID=%d, cmdPlay=%v, connClient.title=%v",
		connClient.transID, cmdPlay, connClient.title)

	if err := connClient.writeMsg(cmdPlay, 0, nil, connClient.title); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

// parseURL takes app, stream name and tcUrl from the RTMP(S) URL and returns the host and
// the port (with leading colon) to connect to.
func (connClient *ConnClient) parseURL(url string) (host, port string, err error) {
	u, err := neturl.Parse(url)
	if err != nil {
		return "", "", err
	}
	connClient.url = url
	path := strings.TrimLeft(u.Path, "/")
	ps := strings.SplitN(path, "/", 2)
	if len(ps) != 2 {
		return "", "", fmt.Errorf("u path err: %s", path)
	}
	connClient.app = ps[0]
	connClient.title = ps[1]
	if u.RawQuery != "" {
		connClient.title += "?" + u.RawQuery
	}
	connClient.isRTMPS = strings.HasPrefix(url, "rtmps://")

	if connClient.isRTMPS {
		connClient.tcurl = "rtmps://" + u.Host + "/" + connClient.app
		port = ":443"
	} else {
		connClient.tcurl = "rtmp://" + u.Host + "/" + connClient.app
		port = ":1935"
	}

	host = u.Host
	if strings.Contains(host, ":") {
		host, port, err = net.SplitHostPort(host)
		if err != nil {
			return "", "", err
		}
		port = ":" + port
	}
	return host, port, nil
}

// resolveRemote resolves host and returns one of its addresses, with port appended for IPv4.
func resolveRemote(ctx context.Context, host, port string) (string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	log.Debugf("ips: %v, host: %v", ips, host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no addresses found for host %s", host)
	}
	//nolint:gosec // G404: only spreads connections across the resolved addresses, not security relevant.
	remoteIP := ips[rand.IntN(len(ips))].IP.String()
	if !strings.Contains(remoteIP, ":") {
		remoteIP += port
	}
	return remoteIP, nil
}

func (connClient *ConnClient) Start(url string, method string) error {
	host, port, err := connClient.parseURL(url)
	if err != nil {
		return err
	}

	ctx := context.Background()
	localIP := ":0"
	remoteIP, err := resolveRemote(ctx, host, port)
	if err != nil {
		log.Warning(err)
		return err
	}

	local, err := net.ResolveTCPAddr("tcp", localIP)
	if err != nil {
		log.Warning(err)
		return err
	}
	log.Debug("remoteIP: ", remoteIP)
	remote, err := net.ResolveTCPAddr("tcp", remoteIP)
	if err != nil {
		log.Warning(err)
		return err
	}

	conn, err := connClient.dial(ctx, local, remote)
	if err != nil {
		log.Warning(err)
		return err
	}

	log.Debug("connection:", "local:", conn.LocalAddr(), "remote:", conn.RemoteAddr())

	connClient.conn = NewConn(conn, 4*1024)

	log.Debug("HandshakeClient....")
	if err := connClient.conn.HandshakeClient(); err != nil {
		return err
	}

	log.Debug("writeConnectMsg....")
	if err := connClient.writeConnectMsg(); err != nil {
		return err
	}
	log.Debug("writeCreateStreamMsg....")
	if err := connClient.writeCreateStreamMsg(); err != nil {
		log.Debug("writeCreateStreamMsg error", err)
		return err
	}

	log.Debug("method control:", method, av.PUBLISH, av.PLAY)
	switch method {
	case av.PUBLISH:
		if err := connClient.writePublishMsg(); err != nil {
			return err
		}
	case av.PLAY:
		if err := connClient.writePlayMsg(); err != nil {
			return err
		}
	}

	return nil
}

// dial opens the TCP or, for rtmps URLs, TLS connection to remote.
func (connClient *ConnClient) dial(ctx context.Context, local, remote *net.TCPAddr) (net.Conn, error) {
	if !connClient.isRTMPS {
		return net.DialTCP("tcp", local, remote)
	}

	config, err := tlsConfig()
	if err != nil {
		return nil, err
	}
	dialer := &tls.Dialer{Config: config}
	return dialer.DialContext(ctx, "tcp", remote.String())
}

// tlsConfig returns the client TLS configuration, verifying against the system roots unless
// enable_tls_verify is turned off.
func tlsConfig() (*tls.Config, error) {
	if !configure.Config.GetBool("enable_tls_verify") {
		//nolint:gosec // G402: verification is disabled explicitly via the enable_tls_verify option.
		return &tls.Config{InsecureSkipVerify: true}, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	return &tls.Config{RootCAs: roots}, nil
}

func (connClient *ConnClient) Write(c ChunkStream) error {
	if c.TypeID == av.TagScriptDataAMF0 ||
		c.TypeID == av.TagScriptDataAMF3 {
		var err error
		if c.Data, err = amf.MetaDataReform(c.Data, amf.ADD); err != nil {
			return err
		}
		if c.Length, err = MessageLength(len(c.Data)); err != nil {
			return err
		}
	}
	return connClient.conn.Write(&c)
}

func (connClient *ConnClient) Flush() error {
	return connClient.conn.Flush()
}

func (connClient *ConnClient) Read(c *ChunkStream) (err error) {
	return connClient.conn.Read(c)
}

func (connClient *ConnClient) GetInfo() (app string, name string, url string) {
	app = connClient.app
	name = connClient.title
	url = connClient.url
	return app, name, url
}

func (connClient *ConnClient) GetStreamID() uint32 {
	return connClient.streamid
}

func (connClient *ConnClient) Close(error) {
	if err := connClient.conn.Close(); err != nil {
		log.Debug("rtmp client close: ", err)
	}
}
