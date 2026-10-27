# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- golangci-lint config (`.golangci.yaml`).
- `make lint`, `make fmt` and `make install-dev` targets.
- `core.MessageLength` to validate RTMP message lengths.
- Dependency review workflow for pull requests.
- Test coverage summary as a PR comment, and a coverage badge in the README.
- Tests for the API JWT middleware.

### Changed

- livego is now a library only.
- Minimum supported Go version is 1.22.
- **Breaking:** exported names use Go naming (e.g. `TAG_AUDIO` → `TagAudio`).
- **Breaking:** `Url`/`Id` names are now `URL`/`ID` (e.g. `StreamID`, `NewID`).
- **Breaking:** `flv.NewFLVWriter` now returns an error.
- **Breaking:** `Conn.SetBegin` and `Conn.SetRecorded` now return an error.
- **Breaking:** API and HLS `Serve` now return the server error.
- **Breaking:** API 403 responses now carry `golang-jwt` error messages
  (e.g. `token has invalid claims: token is expired`).
- Replaced `satori/go.uuid` with `google/uuid`.
- Replaced `dgrijalva/jwt-go` and `auth0/go-jwt-middleware` with `golang-jwt/jwt/v5`.
- CodeQL actions bumped from v3 to v4.
- Stream keys are generated with `crypto/rand`.
- HTTP servers set a 10s `ReadHeaderTimeout`.
- FLV recordings are created with `0600`, folders with `0750`.
- A failed flush now closes the RTMP writer.
- Ignored errors are now handled or logged.

### Fixed

- Default config was stored under the wrong keys.
- HLS crashed when the audio sample rate was unknown.
- Negative composition times broke HLS PTS.
- Oversized FLV tags, ADTS frames and AMF strings no longer get truncated lengths.
- Oversized RTMP messages now return an error.
- Panics on unexpected types in AMF, RTMP and cache code.
- AMF3 dynamic objects wrote the end marker after every member.
- AMF0 ECMA array length errors were ignored.
- Chunk size 0 caused a division by zero.
- The API JWT middleware now rejects tokens with an unexpected algorithm before verifying them.
- Vulnerable dependencies: `golang.org/x/text` (0.3.2 → 0.22.0), `golang.org/x/sys`
  (2019 → 0.30.0) and `gopkg.in/yaml.v2` (2.2.4 → 2.4.0).
- `dgrijalva/jwt-go` access restriction bypass (GO-2020-0017), fixed by the switch to `golang-jwt`.

### Removed

- The `livego` binary (`main.go`).
- Docker image, GoReleaser config and release workflow.

## [0.0.22]

### Fixed

- fixed a out of range panic in the parsing of flv audio headers

## [0.0.21]

### Fixed

- replaced arena allocator (pool.go) with sync.Pool for better memory management ([gwuhaolin/livego#34](https://github.com/gwuhaolin/livego/issues/34), [gwuhaolin/livego#84](https://github.com/gwuhaolin/livego/issues/84), [gwuhaolin/livego#163](https://github.com/gwuhaolin/livego/issues/163), [gwuhaolin/livego#175](https://github.com/gwuhaolin/livego/issues/175))

## [0.0.20]

### Added

- JSON Web Token support.

```json
// livego.json
{
  "jwt": {
    "secret": "testing",
    "algorithm": "HS256"
  },
  "server": [
    {
      "appname": "live",
      "live": true,
      "hls": true
    }
  ]
}
```

- Use redis for store room keys

```json
// livego.json
{
  "redis_addr": "localhost:6379",
  "server": [
    {
      "appname": "live",
      "live": true,
      "hls": true
    }
  ]
}
```

- Makefile

### Changed

- Show `players`.
- Show `stream_id`.
- Deleted keys saved in physical file, now the keys are in cached using `go-cache` by default.
- Using `logrus` like log system.
- Using method `.Get(queryParamName)` to get an url query param.
- Replaced `errors.New(...)` to `fmt.Errorf(...)`.
- Replaced types string on config params `liveon` and `hlson` to booleans `live: true/false` and `hls: true/false`
- Using viper for config, allow use file, cloud providers, environment vars or flags.
- Using yaml config by default.
