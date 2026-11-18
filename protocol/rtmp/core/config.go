package core

import "crypto/tls"

// Config configures RTMP client connections.
type Config struct {
	TLSConfig *tls.Config // TLSConfig is used when dialing rtmps:// URLs; nil verifies against the system roots
}
