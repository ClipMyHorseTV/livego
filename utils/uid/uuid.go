package uid

import (
	"encoding/base64"

	"github.com/google/uuid"
)

func NewID() string {
	id := uuid.New()
	b64 := base64.URLEncoding.EncodeToString(id[:12])
	return b64
}
