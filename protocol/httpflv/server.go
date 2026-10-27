package httpflv

import (
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	handler av.Handler
}

type stream struct {
	Key string `json:"key"`
	ID  string `json:"id"`
}

type streams struct {
	Publishers []stream `json:"publishers"`
	Players    []stream `json:"players"`
}

func NewServer(h av.Handler) *Server {
	return &Server{
		handler: h,
	}
}

func (server *Server) Serve(l net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.handleConn(w, r)
	})
	mux.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		server.getStream(w, r)
	})
	// no write timeout: FLV responses are long-lived streams
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.Serve(l)
}

// 获取发布和播放器的信息.
func (server *Server) getStreams() *streams {
	rtmpStream, ok := server.handler.(*rtmp.RtmpStream)
	if !ok || rtmpStream == nil {
		return nil
	}
	msgs := new(streams)

	rtmpStream.GetStreams().Range(func(key, val any) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		s, ok := val.(*rtmp.Stream)
		if !ok {
			return true
		}
		if s.GetReader() != nil {
			msgs.Publishers = append(msgs.Publishers, stream{k, s.GetReader().Info().UID})
		}
		s.GetWs().Range(func(_, v any) bool {
			if pw, ok := v.(*rtmp.PackWriterCloser); ok && pw.GetWriter() != nil {
				msgs.Players = append(msgs.Players, stream{k, pw.GetWriter().Info().UID})
			}
			return true
		})
		return true
	})

	return msgs
}

func (server *Server) getStream(w http.ResponseWriter, _ *http.Request) {
	msgs := server.getStreams()
	if msgs == nil {
		return
	}
	resp, err := json.Marshal(msgs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(resp); err != nil {
		log.Debug("http flv: write streams response: ", err)
	}
}

func (server *Server) handleConn(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("http flv handleConn panic: ", r)
		}
	}()

	url := r.URL.String()
	u := r.URL.Path
	if pos := strings.LastIndex(u, "."); pos < 0 || u[pos:] != ".flv" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	path := strings.TrimSuffix(strings.TrimLeft(u, "/"), ".flv")
	paths := strings.SplitN(path, "/", 2)
	log.Debug("url:", u, "path:", path, "paths:", paths)

	if len(paths) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// 判断视屏流是否发布,如果没有发布,直接返回404
	msgs := server.getStreams()
	if msgs == nil || !slices.ContainsFunc(msgs.Publishers, func(item stream) bool { return item.Key == path }) {
		http.Error(w, "invalid path", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	writer := NewFLVWriter(paths[0], paths[1], url, w)

	server.handler.HandleWriter(writer)
	writer.Wait()
}
