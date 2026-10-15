package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"path"
	"runtime"
	"time"

	"github.com/ClipMyHorseTV/livego/configure"
	"github.com/ClipMyHorseTV/livego/protocol/api"
	"github.com/ClipMyHorseTV/livego/protocol/hls"
	"github.com/ClipMyHorseTV/livego/protocol/httpflv"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp"

	log "github.com/sirupsen/logrus"
)

var VERSION = "master"

func startHls(config *configure.HLSConfig) (*hls.Server, error) {
	hlsAddr := config.Address
	hlsListen, err := net.Listen("tcp", hlsAddr)
	if err != nil {
		return nil, err
	}

	// TODO: move this into the struct for the HLS Object
	// this will allow to also use WaitGroups to ensure the listener closes
	//
	// The following is a workaround for the time being, yes I know this is not ideal.

	errCh := make(chan error, 1)
	defer close(errCh)

	hlsServer := hls.NewServer()
	go func() {
		log.Infof("Starting HLS listener on addr '%s'", hlsAddr)
		err := hlsServer.Serve(hlsListen)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	time.Sleep(200 * time.Millisecond) // Workaround: Wait a little bit to see if there are startup errors
	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	return hlsServer, nil
}

func startRtmp(config *configure.RTMPConfig, stream *rtmp.RtmpStream, hlsServer *hls.Server) error {
	rtmpAddr := config.Address
	isRTMPS := config.RTMPS != nil

	var rtmpListen net.Listener
	if isRTMPS {
		cert, err := tls.LoadX509KeyPair(config.RTMPS.CertFile, config.RTMPS.KeyFile)
		if err != nil {
			return err
		}

		rtmpListen, err = tls.Listen("tcp", rtmpAddr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err != nil {
			return err
		}

		log.Infof("Started RTMP(s) listener on '%s'", rtmpAddr)
	} else {
		var err error
		rtmpListen, err = net.Listen("tcp", rtmpAddr)
		if err != nil {
			return err
		}

		log.Infof("Started RTMP listener on '%s'", rtmpAddr)
	}

	rtmpServer, err := rtmp.NewRtmpServer(config, stream, hlsServer)
	if err != nil {
		return err
	}
	if hlsServer != nil {
		log.Info("Starting RTMP Server with HLS forwarding")
	} else {
		log.Info("Starting RTMP Server")
	}

	return rtmpServer.Serve(rtmpListen)
}

func startHTTPFlv(config *configure.Config, stream *rtmp.RtmpStream) error {
	httpflvAddr := config.HTTPFLVAddr

	flvListen, err := net.Listen("tcp", httpflvAddr)
	if err != nil {
		return err
	}

	// TODO: move this into the struct for the HTTP-FLV Object
	// this will allow to also use WaitGroups to ensure the listener closes
	//
	// The following is a workaround for the time being, yes I know this is not ideal.

	errCh := make(chan error, 1)
	defer close(errCh)

	httpFlvServer := httpflv.NewServer(stream)
	go func() {
		log.Infof("Starting HTTP-FLV listener on addr '%s'", httpflvAddr)
		err := httpFlvServer.Serve(flvListen)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	time.Sleep(200 * time.Millisecond) // Workaround: Wait a little bit to see if there are startup errors
	select {
	case err := <-errCh:
		return err
	default:
	}

	return nil
}

func startAPI(config *configure.Config, stream *rtmp.RtmpStream) {
	apiAddr := config.APIAddr
	rtmpAddr := config.RTMPAddr

	if apiAddr != "" {
		opListen, err := net.Listen("tcp", apiAddr)
		if err != nil {
			log.Fatal(err)
		}

		opServer := api.NewServer(stream, rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTP-API server panic: ", r)
				}
			}()
			log.Info("HTTP-API listen On ", apiAddr)
			opServer.Serve(opListen)
		}()
	}
}

func main() {
	config, err := configure.InitConfig("")
	if err != nil {
		log.Fatal(err)
	}

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})

	log.Infof(`
     _     _            ____       
    | |   (_)_   _____ / ___| ___  
    | |   | \ \ / / _ \ |  _ / _ \ 
    | |___| |\ V /  __/ |_| | (_) |
    |_____|_| \_/ \___|\____|\___/ 
        version: %s
	`, VERSION)

	for _, app := range config.Server {
		stream := rtmp.NewRtmpStream()
		var hlsServer *hls.Server
		if app.Hls {
			hlsServer = startHls(config)
		}
		if app.FLVViaHTTP {
			startHTTPFlv(config, stream)
		}
		if app.Api {
			startAPI(config, stream)
		}

		startRtmp(config, stream, hlsServer)
	}
}
