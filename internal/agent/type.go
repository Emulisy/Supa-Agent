package agent

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

type Type string

const (
	TypeChatModel   Type = "chat_model"
	TypeDeep        Type = "deep"
	TypeSequential  Type = "sequential"
	TypeParallel    Type = "parallel"
	TypeLoop        Type = "loop"
	TypeSupervisor  Type = "supervisor"
	TypePlanExecute Type = "plan_execute"
)

type Definition struct {
	ID     uint64
	UserID uint64
	Type   Type
	Config json.RawMessage
}

// ModelConfig is shared by model-backed agent configurations. An omitted Model
// selects the gateway's current default at runtime.
type ModelConfig struct {
	Model string `json:"model,omitempty"`
}

// Agent provides type-specific validation and Eino runtime construction.
// Service resolves models at runtime and coordinates persistence for agents.
type Agent interface {
	Validate() error
	Build(ctx context.Context, chatModel model.ToolCallingChatModel) (adk.Agent, error)
}
