package api

import "github.com/golang-jwt/jwt/v5"

type Config struct {
	JWT ConfigJWT
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
