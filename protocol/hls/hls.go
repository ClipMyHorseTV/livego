package hls

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClipMyHorseTV/livego/configure"

	"github.com/ClipMyHorseTV/livego/av"

	log "github.com/sirupsen/logrus"
)

const (
	duration = 3000
)

var (
	ErrNoPublisher         = errors.New("no publisher")
	ErrInvalidReq          = errors.New("invalid req url path")
	ErrNoSupportVideoCodec = errors.New("no support video codec")
	ErrNoSupportAudioCodec = errors.New("no support audio codec")
)

var crossdomainxml = []byte(`<?xml version="1.0" ?>
<cross-domain-policy>
	<allow-access-from domain="*" />
	<allow-http-request-headers-from domain="*" headers="*"/>
</cross-domain-policy>`)

type Server struct {
	listener net.Listener
	conns    *sync.Map
}

func NewServer() *Server {
	ret := &Server{
		conns: &sync.Map{},
	}
	go ret.checkStop()
	return ret
}

func (server *Server) Serve(listener net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.handle(w, r)
	})
	server.listener = listener

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if configure.Config.GetBool("use_hls_https") {
		return srv.ServeTLS(listener, "server.crt", "server.key")
	}
	return srv.Serve(listener)
}

func (server *Server) GetWriter(info av.Info) av.WriteCloser {
	if s := server.getConn(info.Key); s != nil {
		return s
	}
	log.Debug("new hls source")
	s := NewSource(info)
	server.conns.Store(info.Key, s)
	return s
}

func (server *Server) getConn(key string) *Source {
	v, ok := server.conns.Load(key)
	if !ok {
		return nil
	}
	s, ok := v.(*Source)
	if !ok {
		return nil
	}
	return s
}

func (server *Server) checkStop() {
	for {
		<-time.After(5 * time.Second)

		server.conns.Range(func(key, val any) bool {
			v, ok := val.(*Source)
			if !ok {
				return true
			}
			if !v.Alive() && !configure.Config.GetBool("hls_keep_after_end") {
				log.Debug("check stop and remove: ", v.Info())
				server.conns.Delete(key)
			}
			return true
		})
	}
}

func (server *Server) handle(w http.ResponseWriter, r *http.Request) {
	if path.Base(r.URL.Path) == "crossdomain.xml" {
		w.Header().Set("Content-Type", "application/xml")
		writeBody(w, crossdomainxml)
		return
	}
	switch path.Ext(r.URL.Path) {
	case ".m3u8":
		key := server.parseM3u8(r.URL.Path)
		conn := server.getConn(key)
		if conn == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		tsCache := conn.GetCacheInc()
		if tsCache == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		body, err := tsCache.GenM3U8PlayList()
		if err != nil {
			log.Debug("GenM3U8PlayList error: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/x-mpegURL")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writeBody(w, body)
	case ".ts":
		key, _ := server.parseTs(r.URL.Path)
		conn := server.getConn(key)
		if conn == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		tsCache := conn.GetCacheInc()
		item, err := tsCache.GetItem(r.URL.Path)
		if err != nil {
			log.Debug("GetItem error: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "video/mp2ts")
		w.Header().Set("Content-Length", strconv.Itoa(len(item.Data)))
		writeBody(w, item.Data)
	}
}

// writeBody writes a response body; a failed write means the client went away, so it is only logged.
func writeBody(w http.ResponseWriter, body []byte) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	//nolint:gosec // G705: body is a TS segment or M3U8 playlist served with an explicit media Content-Type and nosniff, never HTML.
	if _, err := w.Write(body); err != nil {
		log.Debug("hls: write response: ", err)
	}
}

func (server *Server) parseM3u8(pathstr string) string {
	pathstr = strings.TrimLeft(pathstr, "/")
	return strings.Split(pathstr, path.Ext(pathstr))[0]
}

func (server *Server) parseTs(pathstr string) (key string, err error) {
	pathstr = strings.TrimLeft(pathstr, "/")
	paths := strings.SplitN(pathstr, "/", 3)
	if len(paths) != 3 {
		err = fmt.Errorf("invalid path=%s", pathstr)
		return key, err
	}
	key = paths[0] + "/" + paths[1]

	return key, err
}
