package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/cloudwego/eino/schema"
)

// jsonlSession caches one persisted conversation. ID and CreatedAt are immutable;
// its message operations are safe for concurrent use.
type jsonlSession struct {
	ID        string
	CreatedAt time.Time

	mu       sync.RWMutex
	path     string
	messages []*schema.Message
	deleted  bool
}

// appendMessages commits a batch of complete JSONL records before updating the cache.
// On write/sync failure, truncate back to the previous boundary.
func (s *jsonlSession) appendMessages(ctx context.Context, messages []*schema.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleted {
		return model.ErrSessionDeleted
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var record bytes.Buffer
	owned := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		copy, err := decodeMessage(data)
		if err != nil {
			return err
		}
		record.Write(data)
		record.WriteByte('\n')
		owned = append(owned, copy)
	}
	if len(owned) == 0 {
		return nil
	}
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return errors.Join(err, f.Close())
	}
	n, err := f.Write(record.Bytes())
	if err == nil && n != record.Len() {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		return errors.Join(err, f.Truncate(info.Size()), f.Sync(), f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.messages = append(s.messages, owned...)
	return nil
}

// GetMessages returns the complete chronological history in a copied slice.
// The message objects and their nested fields are shared and must be treated as
// immutable. Numbers in untyped metadata are represented as json.Number. A
// deleted session remains readable as a snapshot, but cannot append.
func (s *jsonlSession) GetMessages() []*schema.Message {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make([]*schema.Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

// Title derives a title locally from the first nonempty user text, capped at 60
// Unicode characters. Sessions without user text are titled "New session".
func (s *jsonlSession) Title() string {
	if s == nil {
		return "New session"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, msg := range s.messages {
		if msg.Role != schema.User {
			continue
		}
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		runes := []rune(text)
		if len(runes) > 60 {
			runes = runes[:60]
		}
		return string(runes)
	}
	return "New session"
}

func decodeMessage(data []byte) (*schema.Message, error) {
	var msg *schema.Message
	decoder := json.NewDecoder(bytes.NewReader(data))
	// Extra fields contain arbitrary JSON. Preserve their numeric precision
	// instead of converting large integers and decimals to float64.
	decoder.UseNumber()
	if err := decoder.Decode(&msg); err != nil {
		return nil, err
	}
	if msg == nil || msg.Role == "" {
		return nil, errors.New("expected a message with a role")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("expected one message per JSONL record")
		}
		return nil, err
	}
	return msg, nil
}
