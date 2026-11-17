package repo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/cloudwego/eino/schema"
)

// SessionRepository preserves the JSONL format, with one store per owner.
// A directory must be owned by exactly one API process.
type SessionRepository struct {
	mu     sync.Mutex
	root   string
	stores map[uint64]*jsonlStore
}

func NewSessionRepository(dir string) (*SessionRepository, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, model.ErrInvalidInput
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &SessionRepository{root: root, stores: make(map[uint64]*jsonlStore)}, nil
}

func (r *SessionRepository) store(ctx context.Context, userID uint64) (*jsonlStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID == 0 {
		return nil, model.ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.stores[userID]; s != nil {
		return s, nil
	}
	s, err := newJSONLStore(filepath.Join(r.root, strconv.FormatUint(userID, 10)))
	if err != nil {
		return nil, err
	}
	r.stores[userID] = s
	return s, nil
}

func (r *SessionRepository) Create(ctx context.Context, userID uint64, id string) (*model.Session, error) {
	s, err := r.store(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.validate(id); err != nil {
		return nil, model.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := createSession(filepath.Join(s.dir, id+".jsonl"), id)
	if err != nil {
		return nil, err
	}
	s.cache[sessionKey(id)] = v
	return sessionSnapshot(v)
}

func (r *SessionRepository) session(ctx context.Context, userID uint64, id string) (*jsonlSession, error) {
	s, err := r.store(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Invalid and other-owner IDs have the same observable result as missing IDs.
	if err := s.validate(id); err != nil {
		return nil, model.ErrSessionNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.cache[sessionKey(id)]; v != nil {
		return v, nil
	}
	v, err := loadSession(filepath.Join(s.dir, id+".jsonl"), id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, model.ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	s.cache[sessionKey(id)] = v
	return v, nil
}

func sessionSnapshot(s *jsonlSession) (*model.Session, error) {
	// Copy nested data so the runtime cannot modify cached or persisted history.
	messages := s.GetMessages()
	owned := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		copy, err := decodeMessage(data)
		if err != nil {
			return nil, err
		}
		owned = append(owned, copy)
	}
	return &model.Session{SessionInfo: model.SessionInfo{ID: s.ID, CreatedAt: s.CreatedAt, Title: s.Title()}, Messages: owned}, nil
}

func (r *SessionRepository) Get(ctx context.Context, userID uint64, id string) (*model.Session, error) {
	s, err := r.session(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return sessionSnapshot(s)
}

func (r *SessionRepository) Append(ctx context.Context, userID uint64, id string, messages []*schema.Message) error {
	s, err := r.session(ctx, userID, id)
	if err != nil {
		return err
	}
	return s.appendMessages(ctx, messages)
}

func (r *SessionRepository) List(ctx context.Context, userID uint64) ([]model.SessionInfo, error) {
	s, err := r.store(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.List()
}

func (r *SessionRepository) Delete(ctx context.Context, userID uint64, id string) error {
	s, err := r.store(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.validate(id); err != nil {
		return model.ErrSessionNotFound
	}
	if err := s.Delete(id); errors.Is(err, os.ErrNotExist) {
		return model.ErrSessionNotFound
	} else {
		return err
	}
}
