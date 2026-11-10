package uid

import (
	"crypto/rand"
	"math/big"
)

var letterRunes = []rune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// RandStringRunes returns a random string of n alphanumeric characters drawn from a
// cryptographically secure source, as it is used for stream keys.
func RandStringRunes(n int) string {
	maxIdx := big.NewInt(int64(len(letterRunes)))
	b := make([]rune, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			panic("uid: crypto/rand failed: " + err.Error())
		}
		b[i] = letterRunes[idx.Int64()]
	}
	return string(b)
}
