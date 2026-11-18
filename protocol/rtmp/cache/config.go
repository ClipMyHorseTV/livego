package cache

// Config configures the per-stream packet cache.
type Config struct {
	GopNum int // GopNum is the number of GOPs cached per stream for new players
}

// DefaultConfig returns the default cache configuration.
func DefaultConfig() Config {
	return Config{GopNum: 1}
}
