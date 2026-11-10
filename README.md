<p align="center">
  <img src="./logo.png" alt="livego" width="200" height="80" />
</p>

<h3 align="center">A simple, efficient live broadcast server library for Go</h3>

<p align="center">
  <a href="https://github.com/ClipMyHorseTV/livego/actions/workflows/test.yml"><img src="https://github.com/ClipMyHorseTV/livego/actions/workflows/test.yml/badge.svg" alt="Test" /></a>
  <a href="https://github.com/ClipMyHorseTV/livego/actions/workflows/test.yml"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/ClipMyHorseTV/livego/badges/coverage.json" alt="Coverage" /></a>
  <a href="https://github.com/ClipMyHorseTV/livego/actions/workflows/codeql.yml"><img src="https://github.com/ClipMyHorseTV/livego/actions/workflows/codeql.yml/badge.svg" alt="CodeQL Advanced" /></a>
  <a href="https://pkg.go.dev/github.com/ClipMyHorseTV/livego"><img src="https://pkg.go.dev/badge/github.com/ClipMyHorseTV/livego.svg" alt="Go Reference" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License" /></a>
</p>

---

> [!NOTE]
> **Why does this fork exist?**
> This fork turns [gwuhaolin/livego](https://github.com/gwuhaolin/livego) into a Go package you can embed in your own programs, rather than a standalone server binary.

## Features

- 🧩 **Embeddable** in your own Go program
- ⚡ **Pure Go**, high performance and cross-platform
- 📡 **Common protocols and formats** supported out of the box

| Transport protocols | Container formats | Encoding formats |
| ------------------- | ----------------- | ---------------- |
| RTMP                | FLV               | H264             |
| AMF                 | TS                | AAC              |
| HLS                 |                   | MP3              |
| HTTP-FLV            |                   |                  |

## Installation

Requires **Go 1.22** or newer.

```bash
go get github.com/ClipMyHorseTV/livego
```

## Quick start

Start an RTMP server that also serves HLS:

```go
package main

import (
	"log"
	"net"

	"github.com/ClipMyHorseTV/livego/protocol/hls"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp"
)

func main() {
	stream := rtmp.NewRtmpStream()

	hlsListener, err := net.Listen("tcp", ":7002")
	if err != nil {
		log.Fatal(err)
	}
	hlsServer := hls.NewServer()
	go hlsServer.Serve(hlsListener)

	rtmpListener, err := net.Listen("tcp", ":1935")
	if err != nil {
		log.Fatal(err)
	}
	rtmpServer := rtmp.NewRtmpServer(stream, hlsServer)
	log.Fatal(rtmpServer.Serve(rtmpListener))
}
```

HTTP-FLV playback and the HTTP management API are added the same way:

| Package   | Constructor                       | Typical address |
| --------- | --------------------------------- | --------------- |
| `rtmp`    | `rtmp.NewRtmpServer(stream, hls)` | `:1935`         |
| `httpflv` | `httpflv.NewServer(stream)`       | `:7001`         |
| `hls`     | `hls.NewServer()`                 | `:7002`         |
| `api`     | `api.NewServer(stream, rtmpAddr)` | `:8090`         |

## Streaming

With the servers running:

1. **Get a channel key** (used to push the stream) from the API:

   ```text
   http://localhost:8090/control/get?room=movie
   ```

2. **Push a stream** to `rtmp://localhost:1935/{appname}/{channelkey}` (the default `appname` is `live`), for example with ffmpeg and the [demo FLV](https://s3plus.meituan.net/v1/mss_7e425c4d9dcb4bb4918bbfa2779e6de1/mpack/default/demo.flv):

   ```bash
   ffmpeg -re -i demo.flv -c copy -f flv rtmp://localhost:1935/{appname}/{channelkey}
   ```

3. **Play it back** with any of:

   | Protocol | URL                                          |
   | -------- | -------------------------------------------- |
   | RTMP     | `rtmp://localhost:1935/{appname}/movie`      |
   | HTTP-FLV | `http://127.0.0.1:7001/{appname}/movie.flv`  |
   | HLS      | `http://127.0.0.1:7002/{appname}/movie.m3u8` |

## Configuration

Settings are read through `configure.Config`, in this order:

1. Built-in defaults
2. A `livego.yaml` in the working directory (see [livego.yaml](./livego.yaml) for the available keys)
3. Environment variables

> [!WARNING]
> Importing any package that depends on `configure` currently also registers and parses livego's command-line flags.

## License

[MIT](./LICENSE). Based on [gwuhaolin/livego](https://github.com/gwuhaolin/livego).
