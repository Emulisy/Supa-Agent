package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Emulisy/Supa-Agent/internal/agent"
	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/cloudwego/eino/schema"
)

type SessionRepository interface {
	Create(context.Context, uint64, string) (*model.Session, error)
	Get(context.Context, uint64, string) (*model.Session, error)
	Append(context.Context, uint64, string, []*schema.Message) error
	List(context.Context, uint64) ([]model.SessionInfo, error)
	Delete(context.Context, uint64, string) error
}

type RunRepository interface {
	Create(context.Context, model.Run) error
	Get(context.Context, uint64, string) (*model.Run, error)
	Append(context.Context, model.Run, model.RunEvent) error
	Finish(context.Context, model.Run, model.RunStatus, string) error
	Read(context.Context, string, string, time.Duration) ([]model.RunEvent, error)
	Recover(context.Context) error
}

type AgentLookup interface {
	FindByID(context.Context, uint64, uint64) (*model.Definition, error)
}
type Runtime interface {
	Prepare(context.Context, *model.Definition) (agent.Execution, error)
}

type ChatConfig struct {
	RunTimeout    time.Duration
	MaxConcurrent int
}
type ChatInput struct {
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
}
type sessionKey struct {
	userID uint64
	id     string
}
type activeRun struct {
	cancel context.CancelFunc
	key    sessionKey
}

type ChatService struct {
	agents   AgentLookup
	runtime  Runtime
	sessions SessionRepository
	runs     RunRepository
	cfg      ChatConfig
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	active   map[string]activeRun
	busy     map[sessionKey]bool
	wg       sync.WaitGroup
}

func NewChatService(ctx context.Context, agents AgentLookup, runtime Runtime, sessions SessionRepository, runs RunRepository, cfg ChatConfig) (*ChatService, error) {
	if agents == nil || runtime == nil || sessions == nil || runs == nil {
		return nil, errors.New("chat: all dependencies are required")
	}
	if cfg.RunTimeout <= 0 || cfg.MaxConcurrent < 1 {
		return nil, errors.New("chat: positive timeout and capacity are required")
	}
	recoveryCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if err := runs.Recover(recoveryCtx); err != nil {
		return nil, err
	}
	appCtx, cancel := context.WithCancel(ctx)
	return &ChatService{agents: agents, runtime: runtime, sessions: sessions, runs: runs, cfg: cfg, ctx: appCtx, cancel: cancel, active: make(map[string]activeRun), busy: make(map[sessionKey]bool)}, nil
}

func (s *ChatService) Start(ctx context.Context, userID, agentID uint64, input ChatInput) (*model.Run, error) {
	if userID == 0 || !utf8.ValidString(input.Content) || strings.TrimSpace(input.Content) == "" || len(input.Content) > 64<<10 {
		return nil, model.ErrInvalidInput
	}
	def, err := s.agents.FindByID(ctx, userID, agentID)
	if err != nil {
		return nil, err
	}
	prepared, err := s.runtime.Prepare(ctx, def)
	if err != nil {
		return nil, err
	}
	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = rand.Text()
	} else {
		existing, err := s.sessions.Get(ctx, userID, sessionID)
		if err != nil {
			return nil, err
		}
		sessionID = existing.ID
	}
	run := model.Run{ID: rand.Text(), UserID: userID, AgentID: agentID, SessionID: sessionID, Status: model.RunRunning}
	run.EventsURL = "/runs/" + run.ID + "/events"
	key := sessionKey{userID, sessionID}
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil, model.ErrUnavailable
	}
	if s.busy[key] {
		s.mu.Unlock()
		return nil, model.ErrConflict
	}
	if len(s.active) >= s.cfg.MaxConcurrent {
		s.mu.Unlock()
		return nil, model.ErrCapacity
	}
	runCtx, cancel := context.WithTimeout(s.ctx, s.cfg.RunTimeout)
	s.active[run.ID], s.busy[key] = activeRun{cancel: cancel, key: key}, true
	s.wg.Add(1)
	s.mu.Unlock()
	launched := false
	defer func() {
		if !launched {
			s.release(run.ID)
		}
	}()
	var session *model.Session
	if input.SessionID == "" {
		session, err = s.sessions.Create(ctx, userID, sessionID)
	} else {
		session, err = s.sessions.Get(ctx, userID, sessionID)
	}
	if err != nil {
		return nil, err
	}
	if err := runCtx.Err(); err != nil {
		return nil, err
	}
	if err := s.runs.Create(ctx, run); err != nil {
		return nil, err
	}
	userMessage := schema.UserMessage(input.Content)
	if err := s.sessions.Append(ctx, userID, sessionID, []*schema.Message{userMessage}); err != nil {
		s.finish(run, model.RunFailed, "conversation storage failed")
		return nil, err
	}
	history := append(session.Messages, userMessage)
	launched = true
	go s.execute(runCtx, run, prepared, history)
	return &run, nil
}

func (s *ChatService) execute(ctx context.Context, run model.Run, prepared agent.Execution, history []*schema.Message) {
	defer s.release(run.ID)
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("agent run panicked", "run_id", run.ID)
			s.finish(run, model.RunFailed, "agent execution failed")
		}
	}()
	messages, err := prepared.Run(ctx, history, func(event model.RunEvent) error { return s.runs.Append(ctx, run, event) })
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = s.sessions.Append(ctx, run.UserID, run.SessionID, messages)
	}
	if err != nil {
		status, message := model.RunFailed, "agent execution failed"
		if errors.Is(err, context.Canceled) {
			status, message = model.RunCanceled, "run canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			message = "run timed out"
		}
		slog.Error("agent run failed", "run_id", run.ID, "error", err)
		s.finish(run, status, message)
		return
	}
	if err := prepared.Remember(ctx, history[len(history)-1].Content); err != nil {
		slog.Warn("update agent memory failed", "run_id", run.ID, "error", err)
	}
	s.finish(run, model.RunSucceeded, "")
}

func (s *ChatService) finish(run model.Run, status model.RunStatus, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.runs.Finish(ctx, run, status, message); err != nil && !errors.Is(err, model.ErrRunFinished) {
		slog.Error("record run termination failed", "run_id", run.ID, "error", err)
	}
}

func (s *ChatService) release(id string) {
	s.mu.Lock()
	active := s.active[id]
	active.cancel()
	delete(s.active, id)
	delete(s.busy, active.key)
	s.mu.Unlock()
	s.wg.Done()
}

func (s *ChatService) GetRun(ctx context.Context, userID uint64, id string) (*model.Run, error) {
	run, err := s.runs.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	_, active := s.active[id]
	s.mu.Unlock()
	// A failed terminal write can be repaired once Redis becomes available again.
	if run.Status == model.RunRunning && !active {
		if err := s.runs.Finish(ctx, *run, model.RunFailed, "agent execution interrupted"); err != nil && !errors.Is(err, model.ErrRunFinished) {
			return nil, err
		}
		return s.runs.Get(ctx, userID, id)
	}
	return run, nil
}

func (s *ChatService) CancelRun(ctx context.Context, userID uint64, id string) (*model.Run, error) {
	run, err := s.GetRun(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if active, ok := s.active[id]; ok {
		active.cancel()
	}
	s.mu.Unlock()
	return run, nil
}

var cursorPattern = regexp.MustCompile(`^[0-9]+-[0-9]+$`)

func ValidateCursor(after string) error {
	if !cursorPattern.MatchString(after) {
		return fmt.Errorf("%w: invalid event cursor", model.ErrInvalidInput)
	}
	for _, part := range strings.Split(after, "-") {
		if _, err := strconv.ParseUint(part, 10, 64); err != nil {
			return fmt.Errorf("%w: invalid event cursor", model.ErrInvalidInput)
		}
	}
	return nil
}

// Events fetches immediately before blocking, so terminal replay never waits.
func (s *ChatService) Events(ctx context.Context, userID uint64, id, after string, block time.Duration) ([]model.RunEvent, bool, error) {
	if err := ValidateCursor(after); err != nil {
		return nil, false, err
	}
	run, err := s.GetRun(ctx, userID, id)
	if err != nil {
		return nil, false, err
	}
	events, err := s.runs.Read(ctx, id, after, -1)
	if err != nil {
		return nil, false, err
	}
	if len(events) == 0 && run.Status == model.RunRunning && block > 0 {
		events, err = s.runs.Read(ctx, id, after, block)
	}
	if err != nil {
		return nil, false, err
	}
	return events, run.Status != model.RunRunning && len(events) == 0, nil
}

func (s *ChatService) ListSessions(ctx context.Context, userID uint64) ([]model.SessionInfo, error) {
	return s.sessions.List(ctx, userID)
}
func (s *ChatService) GetSession(ctx context.Context, userID uint64, id string) (*model.Session, error) {
	return s.sessions.Get(ctx, userID, id)
}

func (s *ChatService) DeleteSession(ctx context.Context, userID uint64, id string) error {
	session, err := s.sessions.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	key := sessionKey{userID, session.ID}
	s.mu.Lock()
	if s.busy[key] {
		s.mu.Unlock()
		return model.ErrConflict
	}
	s.busy[key] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.busy, key); s.mu.Unlock() }()
	return s.sessions.Delete(ctx, userID, session.ID)
}

// Shutdown prevents new reservations before waiting; no Add can race Wait.
func (s *ChatService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
