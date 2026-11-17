package roomkey

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("roomkey: not found")

// Store maps channels to stream keys and back.
type Store interface {
	// ResetKey generates a new key for channel and invalidates the previous one.
	ResetKey(ctx context.Context, channel string) (key string, err error)
	// GetOrCreateKey returns the key of channel, creating one if none exists.
	GetOrCreateKey(ctx context.Context, channel string) (key string, err error)
	// Channel resolves key to its channel. It returns ErrNotFound if the key is unknown.
	Channel(ctx context.Context, key string) (channel string, err error)
	// DeleteChannel removes channel and its key. It returns ErrNotFound if the channel is unknown.
	DeleteChannel(ctx context.Context, channel string) error
}
