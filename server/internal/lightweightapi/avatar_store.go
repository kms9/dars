package lightweightapi

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

// AvatarObjectStore persists Agent/Squad avatar bytes behind unguessable keys.
// The default implementation is an in-memory store used until durable object
// storage is wired; Workspace delete always goes through Delete so cleanup is
// explicit and testable.
type AvatarObjectStore interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
	Get(ctx context.Context, key string) (contentType string, body []byte, err error)
	Delete(ctx context.Context, key string) error
}

type memoryAvatarStore struct {
	mu   sync.Mutex
	data map[string]avatarObject
}

type avatarObject struct {
	contentType string
	body        []byte
}

func newMemoryAvatarStore() *memoryAvatarStore {
	return &memoryAvatarStore{data: make(map[string]avatarObject)}
}

func (s *memoryAvatarStore) Put(_ context.Context, key, contentType string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := append([]byte(nil), body...)
	s.data[key] = avatarObject{contentType: contentType, body: copied}
	return nil
}

func (s *memoryAvatarStore) Get(_ context.Context, key string) (string, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, ok := s.data[key]
	if !ok {
		return "", nil, errAvatarNotFound
	}
	return obj.contentType, append([]byte(nil), obj.body...), nil
}

func (s *memoryAvatarStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

var errAvatarNotFound = errors.New("avatar_not_found")

func (h *Handler) avatarStore() AvatarObjectStore {
	if h.cfg.AvatarStore != nil {
		return h.cfg.AvatarStore
	}
	return h.avatars
}

func (h *Handler) cleanupAvatarObjects(ctx context.Context, urls []string) error {
	store := h.avatarStore()
	var first error
	for _, url := range urls {
		key := avatarKeyFromURL(url)
		if key == "" {
			continue
		}
		if err := store.Delete(ctx, key); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func avatarKeyFromURL(url string) string {
	const prefix = "/media/avatars/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	return strings.TrimPrefix(url, prefix)
}

func validTextStrings(values []pgtype.Text) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.Valid && value.String != "" {
			out = append(out, value.String)
		}
	}
	return out
}
