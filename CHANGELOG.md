# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
