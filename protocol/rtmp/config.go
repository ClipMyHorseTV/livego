package rtmp

import (
	"slices"
	"time"

	"github.com/ClipMyHorseTV/livego/protocol/rtmp/cache"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"
	"github.com/ClipMyHorseTV/livego/protocol/rtmp/rtmprelay"
)

type Config struct {
	NoAuth bool // NoAuth lets publishers push with the room name instead of its stream key

	ReadTimeout  time.Duration // ReadTimeout is the inactivity timeout of a stream reader
	WriteTimeout time.Duration // WriteTimeout is the inactivity timeout of a stream writer

	Apps   []string // Apps lists the application names that accept RTMP connections
	FLVDir string   // FLVDir enables archiving published streams as FLV files below this directory when set

	Core  core.Config      // Core configures outgoing RTMP connections (client, static pushes)
	Cache cache.Config     // Cache configures the per-stream packet cache
	Relay rtmprelay.Config // Relay configures static pushes
}

// DefaultConfig returns the defaults of the former configure package.
func DefaultConfig() Config {
	return Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Apps:         []string{"live"},
		Cache:        cache.DefaultConfig(),
	}
}

// CheckAppName reports whether appname is a configured application.
// Being listed in Apps replaces the old per-application "live" flag.
func (c *Config) CheckAppName(appname string) bool {
	return slices.Contains(c.Apps, appname)
}
