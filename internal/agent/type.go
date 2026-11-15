package agent

import (
	"context"
	"encoding/json"

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
	ID     uint64
	UserID uint64
	Type   Type
	Config json.RawMessage
}

// Agent provides type-specific validation and Eino runtime construction.
// Service coordinates persistence so concrete agents do not depend on a database.
type Agent interface {
	Validate() error
	Build(ctx context.Context) (adk.Agent, error)
}
