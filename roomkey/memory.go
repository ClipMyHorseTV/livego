package roomkey

import (
	"context"
	"sync"

	"github.com/ClipMyHorseTV/livego/utils/uid"
)

const defaultKeyLen = 48

// MemoryStore is an in-process Store. Keys are lost when the process exits.
type MemoryStore struct {
	mu        sync.Mutex
	byChannel map[string]string
	byKey     map[string]string
	keyLen    int
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore returns an empty MemoryStore that generates 48 character keys.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byChannel: make(map[string]string),
		byKey:     make(map[string]string),
		keyLen:    defaultKeyLen,
	}
}

func (m *MemoryStore) ResetKey(_ context.Context, channel string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setKeyLocked(channel), nil
}

func (m *MemoryStore) GetOrCreateKey(_ context.Context, channel string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key, ok := m.byChannel[channel]; ok {
		return key, nil
	}
	return m.setKeyLocked(channel), nil
}

func (m *MemoryStore) Channel(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	channel, ok := m.byKey[key]
	if !ok {
		return "", ErrNotFound
	}
	return channel, nil
}

func (m *MemoryStore) DeleteChannel(_ context.Context, channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.byChannel[channel]
	if !ok {
		return ErrNotFound
	}
	delete(m.byChannel, channel)
	delete(m.byKey, key)
	return nil
}

// setKeyLocked assigns a fresh unique key to channel and drops its previous key. m.mu must be held.
func (m *MemoryStore) setKeyLocked(channel string) string {
	if old, ok := m.byChannel[channel]; ok {
		delete(m.byKey, old)
	}
	var key string
	for {
		key = uid.RandStringRunes(m.keyLen)
		if _, taken := m.byKey[key]; !taken {
			break
		}
	}
	m.byChannel[channel] = key
	m.byKey[key] = channel
	return key
}
