package hls

type Config struct {
	Encrypted    bool // Encrypted enabled and disabled the serving of the CRT and Key for TLS delivery
	KeepAfterEnd bool // KeepAfterEnd decides if segments should be kept or not after a publisher disconnects
}
