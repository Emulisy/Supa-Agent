package repo

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

// jsonlStore manages persisted sessions and caches their histories in memory. Share
// one jsonlStore per directory; concurrent processes or Stores writing to the same
// directory are not supported. No file handles are kept open between operations.
// On Windows, session IDs follow the filesystem's case-insensitive behavior.
type jsonlStore struct {
	mu    sync.Mutex
	dir   string
	cache map[string]*jsonlSession
}

type sessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// newJSONLStore creates dir if necessary. Relative paths are resolved at construction
// so a later change of working directory cannot redirect session operations.
func newJSONLStore(dir string) (*jsonlStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("memory: storage directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("memory: resolve storage directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("memory: create storage directory: %w", err)
	}
	return &jsonlStore{dir: abs, cache: make(map[string]*jsonlSession)}, nil
}

// List includes uncached sessions on disk, newest first, with ID ascending for
// equal creation times. A malformed session returns an error, not a partial list.
func (s *jsonlStore) List() ([]model.SessionInfo, error) {
	if s == nil || s.dir == "" {
		return nil, errors.New("memory: store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("memory: list sessions: %w", err)
	}
	sessions := make([]model.SessionInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if err := validateID(id); err != nil {
			return nil, fmt.Errorf("memory: list sessions: %w", err)
		}
		key := sessionKey(id)
		session := s.cache[key]
		if session == nil {
			session, err = loadSession(filepath.Join(s.dir, entry.Name()), id)
			if err != nil {
				return nil, err
			}
			s.cache[key] = session
		}
		sessions = append(sessions, model.SessionInfo{
			ID: session.ID, CreatedAt: session.CreatedAt, Title: session.Title(),
		})
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	return sessions, nil
}

// Delete removes a session file and invalidates any cached handle for appends.
// Missing sessions return an error wrapping os.ErrNotExist. Cached handles
// become invalid for appends even if a file with the same ID is later created.
func (s *jsonlStore) Delete(id string) error {
	if err := s.validate(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(id)
	session := s.cache[key]
	if session != nil {
		session.mu.Lock()
		defer session.mu.Unlock()
	}
	err := os.Remove(filepath.Join(s.dir, id+".jsonl"))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		if session != nil {
			session.deleted = true
		}
		delete(s.cache, key)
	}
	if err != nil {
		return fmt.Errorf("memory: delete session %q: %w", id, err)
	}
	return nil
}

func (s *jsonlStore) validate(id string) error {
	if s == nil || s.dir == "" || s.cache == nil {
		return errors.New("memory: store is not initialized")
	}
	return validateID(id)
}

func validateID(id string) error {
	invalid := id == "" || id == "." || !utf8.ValidString(id) ||
		!filepath.IsLocal(id) || filepath.Base(id) != id ||
		strings.TrimSpace(id) != id || strings.HasSuffix(id, ".") ||
		strings.ContainsAny(id, `/\<>:"|?*`)
	for _, r := range id {
		invalid = invalid || unicode.IsControl(r)
	}
	// Reject Windows device names on every platform, including names with an
	// extension, so sessions remain portable between operating systems.
	base, _, _ := strings.Cut(strings.ToUpper(id), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		invalid = true
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		suffix := base[3:]
		if strings.Contains("123456789¹²³", suffix) && utf8.RuneCountInString(suffix) == 1 {
			invalid = true
		}
	}
	if invalid {
		return fmt.Errorf("memory: invalid session ID %q: expected a nonempty filename component", id)
	}
	return nil
}

func sessionKey(id string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(id)
	}
	return id
}

func createSession(path, id string) (*jsonlSession, error) {
	header := sessionHeader{Type: "session", ID: id, CreatedAt: time.Now().UTC()}
	data, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("memory: encode session %q header: %w", id, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("memory: create session %q: %w", id, err)
	}
	if err := writeRecordAndClose(file, data); err != nil {
		return nil, fmt.Errorf("memory: create session %q: %w", id, errors.Join(err, os.Remove(path)))
	}
	return &jsonlSession{ID: id, CreatedAt: header.CreatedAt, path: path}, nil
}

func loadSession(path, id string) (*jsonlSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("memory: open session %q: %w", id, err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	session := &jsonlSession{path: path}
	for line := 1; ; line++ {
		// ReadBytes has no Scanner-style token limit, including for media or
		// large tool responses. Every record must end with a newline.
		data, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(data) == 0 && line > 1 {
			return session, nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = fmt.Errorf("missing header or unterminated JSONL record: %w", io.ErrUnexpectedEOF)
			}
			return nil, fmt.Errorf("memory: load session %q line %d: %w", id, line, err)
		}
		if line == 1 {
			var header sessionHeader
			err = json.Unmarshal(data, &header)
			if err == nil && (header.Type != "session" || sessionKey(header.ID) != sessionKey(id) || header.CreatedAt.IsZero()) {
				err = errors.New("expected a session header with matching ID and creation time")
			}
			if err != nil {
				return nil, fmt.Errorf("memory: load session %q line 1: %w", id, err)
			}
			session.ID, session.CreatedAt = header.ID, header.CreatedAt
			continue
		}
		msg, err := decodeMessage(data)
		if err != nil {
			return nil, fmt.Errorf("memory: load session %q line %d: %w", id, line, err)
		}
		session.messages = append(session.messages, msg)
	}
}

// writeRecordAndClose always closes file, including after a failed write or sync.
func writeRecordAndClose(file *os.File, data []byte) error {
	record := append(data, '\n')
	n, err := file.Write(record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
