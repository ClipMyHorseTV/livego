package api

import (
	"github.com/golang-jwt/jwt/v5"

	"github.com/ClipMyHorseTV/livego/protocol/rtmp/core"
)

type Config struct {
	JWT ConfigJWT

	Relay core.Config // Relay configures the connections of pull/push relays
}

type ConfigJWT struct {
	Secret    string
	Algorithm *jwt.SigningMethodHMAC
}

func DefaultConfig() Config {
	return Config{
		JWT: ConfigJWT{
			Algorithm: jwt.SigningMethodHS256,
		},
	}
}
