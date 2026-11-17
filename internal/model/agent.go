package model

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
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
	ID     uint64          `json:"id"`
	UserID uint64          `json:"user_id"`
	Type   Type            `json:"type"`
	Config json.RawMessage `json:"config"`
}

// ModelConfig is shared by model-backed agent configurations. An omitted Model
// selects the gateway's current default at runtime.
type ModelConfig struct {
	Model string `json:"model,omitempty"`
}

func ValidateDefinition(definition *Definition) error {
	if definition == nil {
		return fmt.Errorf("%w: definition is required", ErrInvalidDefinition)
	}
	if definition.UserID == 0 {
		return fmt.Errorf("%w: user ID is required", ErrInvalidDefinition)
	}
	switch definition.Type {
	case TypeChatModel, TypeDeep, TypeSequential, TypeParallel, TypeLoop, TypeSupervisor, TypePlanExecute:
	default:
		return fmt.Errorf("%w: unsupported type %q", ErrInvalidDefinition, definition.Type)
	}
	if !utf8.Valid(definition.Config) || !json.Valid(definition.Config) {
		return fmt.Errorf("%w: config must contain valid UTF-8 JSON", ErrInvalidDefinition)
	}
	return nil
}
