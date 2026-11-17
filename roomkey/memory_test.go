package roomkey

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStoreGetOrCreateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	k1, err := s.GetOrCreateKey(ctx, "room")
	require.NoError(t, err)
	assert.Len(t, k1, defaultKeyLen)

	k2, err := s.GetOrCreateKey(ctx, "room")
	require.NoError(t, err)
	assert.Equal(t, k1, k2)

	ch, err := s.Channel(ctx, k1)
	require.NoError(t, err)
	assert.Equal(t, "room", ch)
}

func TestMemoryStoreResetInvalidatesOldKey(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	old, err := s.GetOrCreateKey(ctx, "room")
	require.NoError(t, err)
	fresh, err := s.ResetKey(ctx, "room")
	require.NoError(t, err)
	assert.NotEqual(t, old, fresh)

	_, err = s.Channel(ctx, old)
	require.ErrorIs(t, err, ErrNotFound)

	ch, err := s.Channel(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, "room", ch)
}

func TestMemoryStoreDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	require.ErrorIs(t, s.DeleteChannel(ctx, "room"), ErrNotFound)

	key, err := s.GetOrCreateKey(ctx, "room")
	require.NoError(t, err)
	require.NoError(t, s.DeleteChannel(ctx, "room"))

	_, err = s.Channel(ctx, key)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, s.DeleteChannel(ctx, "room"), ErrNotFound)
}

func TestMemoryStoreChannelUnknownKey(t *testing.T) {
	_, err := NewMemoryStore().Channel(context.Background(), "nope")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStoreConcurrent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room := fmt.Sprintf("room-%d", i%5)
			key, _ := s.GetOrCreateKey(ctx, room)
			_, _ = s.Channel(ctx, key)
			_, _ = s.ResetKey(ctx, room)
			_ = s.DeleteChannel(ctx, room)
		}(i)
	}
	wg.Wait()
}
