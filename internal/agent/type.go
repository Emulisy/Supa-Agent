package agent

import (
	"encoding/json"
	"context"

    "github.com/cloudwego/eino/adk"
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
    ID          uint64
    UserID      uint64
    Type        Type
    Config      json.RawMessage
}

type Agent interface {
    // Common database operations
    Save(ctx context.Context) error
    Update(ctx context.Context) error
    Delete(ctx context.Context) error

    // Agent-specific behavior
    Validate() error
    Build(ctx context.Context) (adk.Agent, error)
}