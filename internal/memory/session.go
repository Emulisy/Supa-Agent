// Package memory persists complete conversation histories in local JSONL files.
// Use one shared Store per directory within a single process.
package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

// ErrSessionDeleted means a handle belongs to a session removed by Store.Delete.
var ErrSessionDeleted = errors.New("memory: session has been deleted")

// Session is one persisted conversation. Obtain it through Store.GetOrCreate;
// callers must not change ID or CreatedAt. Its methods are safe for concurrent use.
type Session struct {
	ID        string
	CreatedAt time.Time

	mu       sync.RWMutex
	path     string
	messages []*schema.Message
	deleted  bool
}

// SessionInfo describes a conversation without exposing its message history.
type SessionInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Title     string    `json:"title"`
}

// Append persists a complete message before adding an owned copy to history.
// The caller may change msg after Append returns. Stored messages must have a
// role and be JSON-serializable. Encoding or filesystem failures leave the cached
// history unchanged. Each successful append is synced and closed before return.
func (s *Session) Append(msg *schema.Message) error {
	if s == nil || s.path == "" {
		return errors.New("memory: session is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleted {
		return fmt.Errorf("memory: append to session %q: %w", s.ID, ErrSessionDeleted)
	}
	if msg == nil {
		return errors.New("memory: cannot append a nil message")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("memory: encode message for session %q: %w", s.ID, err)
	}
	// Decode the persisted representation so caller-owned nested fields cannot
	// change history, and cached messages match those loaded after a restart.
	owned, err := decodeMessage(data)
	if err != nil {
		return fmt.Errorf("memory: encode message for session %q: %w", s.ID, err)
	}
	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("memory: append to session %q: %w", s.ID, err)
	}
	if err := writeRecordAndClose(file, data); err != nil {
		return fmt.Errorf("memory: append to session %q: %w", s.ID, err)
	}
	s.messages = append(s.messages, owned)
	return nil
}

// GetMessages returns the complete chronological history in a copied slice.
// The message objects and their nested fields are shared and must be treated as
// immutable. Numbers in untyped metadata are represented as json.Number. A
// deleted session remains readable as a snapshot, but cannot append.
func (s *Session) GetMessages() []*schema.Message {
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
func (s *Session) Title() string {
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
