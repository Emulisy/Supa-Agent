package model

import "github.com/cloudwego/eino/schema"

type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCanceled  RunStatus = "canceled"
)

// UserID is persistence-only; ownership never comes from a request body.
type Run struct {
	ID        string    `json:"run_id"`
	UserID    uint64    `json:"-"`
	AgentID   uint64    `json:"agent_id"`
	SessionID string    `json:"session_id"`
	Status    RunStatus `json:"status"`
	EventsURL string    `json:"events_url"`
}

type RunEventType string

const (
	RunEventStarted      RunEventType = "run_started"
	RunEventMessageDelta RunEventType = "message_delta"
	RunEventMessage      RunEventType = "message"
	RunEventMessageEnd   RunEventType = "message_end"
	RunEventError        RunEventType = "error"
	RunEventDone         RunEventType = "done"
)

type RunEvent struct {
	ID        string          `json:"id,omitempty"`
	Type      RunEventType    `json:"type"`
	RunID     string          `json:"run_id,omitempty"`
	AgentID   uint64          `json:"agent_id,omitempty"`
	MessageID uint64          `json:"message_id,omitempty"`
	AgentName string          `json:"agent_name,omitempty"`
	RunPath   []string        `json:"run_path,omitempty"`
	Role      schema.RoleType `json:"role,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Message   *schema.Message `json:"message,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func (e RunEvent) Terminal() bool { return e.Type == RunEventDone || e.Type == RunEventError }
