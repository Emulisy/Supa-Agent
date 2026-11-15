package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

// ChatModelResolver selects an initialized model without making provider calls.
// The project's llm.Gateway implements this interface; an empty name selects its
// configured default model.
type ChatModelResolver interface {
	Model(name string) (model.ToolCallingChatModel, error)
}

// ChatModelConfig is the JSON configuration stored in Definition.Config.
// For example: {"name":"assistant","instruction":"Be helpful."} uses the
// gateway's default model and Eino's default iteration limit.
type ChatModelConfig struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Instruction   string `json:"instruction,omitempty"`
	Model         string `json:"model,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`
}

// ChatModelAgent validates a persisted definition and builds an Eino agent.
// Persistence is coordinated separately by Service. Definition remains owned by
// the caller, so configuration is parsed again for every validation and build.
type ChatModelAgent struct {
	Definition *Definition
	models     ChatModelResolver
}

var _ Agent = (*ChatModelAgent)(nil)

// NewChatModelAgent checks configuration and dependencies without resolving a
// model or calling a provider. Validate and Build check model availability.
func NewChatModelAgent(definition *Definition, models ChatModelResolver) (*ChatModelAgent, error) {
	if chatModelDependencyMissing(models) {
		return nil, errors.New("agent: chat model resolver is required")
	}
	a := &ChatModelAgent{Definition: definition, models: models}
	if _, err := a.configuration(); err != nil {
		return nil, err
	}
	return a, nil
}

// Validate checks the current definition, JSON configuration, and model selector.
// It does not build an agent, write to the database, or invoke the model.
func (a *ChatModelAgent) Validate() error {
	_, _, err := a.resolveModel()
	return err
}

// Build creates a fresh Eino ChatModelAgent using the current configuration and
// the shared model resolver. It performs no model generation or persistence.
func (a *ChatModelAgent) Build(ctx context.Context) (adk.Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	config, chatModel, err := a.resolveModel()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	built, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   config.Description,
		Instruction:   config.Instruction,
		Model:         chatModel,
		MaxIterations: config.MaxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	return built, nil
}

func (a *ChatModelAgent) configuration() (*ChatModelConfig, error) {
	if a == nil {
		return nil, fmt.Errorf("%w: chat model agent is not initialized", ErrInvalidDefinition)
	}
	if err := validateDefinition(a.Definition); err != nil {
		return nil, err
	}
	if a.Definition.Type != TypeChatModel {
		return nil, fmt.Errorf("%w: chat model agent requires type %q", ErrInvalidDefinition, TypeChatModel)
	}
	decoder := json.NewDecoder(bytes.NewReader(a.Definition.Config))
	decoder.DisallowUnknownFields()
	var config *ChatModelConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%w: decode chat model config: %v", ErrInvalidDefinition, err)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: chat model config must be an object", ErrInvalidDefinition)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: chat model config must contain one JSON object", ErrInvalidDefinition)
	}
	if strings.TrimSpace(config.Name) == "" {
		return nil, fmt.Errorf("%w: chat model name is required", ErrInvalidDefinition)
	}
	if config.Model != "" && strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("%w: chat model selector must not be whitespace", ErrInvalidDefinition)
	}
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("%w: max_iterations must be non-negative", ErrInvalidDefinition)
	}
	return config, nil
}

func (a *ChatModelAgent) resolveModel() (*ChatModelConfig, model.ToolCallingChatModel, error) {
	config, err := a.configuration()
	if err != nil {
		return nil, nil, err
	}
	if chatModelDependencyMissing(a.models) {
		return nil, nil, errors.New("agent: chat model resolver is required")
	}
	chatModel, err := a.models.Model(config.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: resolve chat model: %w", err)
	}
	if chatModelDependencyMissing(chatModel) {
		return nil, nil, errors.New("agent: model resolver returned a nil chat model")
	}
	return config, chatModel, nil
}

// Interface values may contain a nil pointer (for example, *llm.Gateway).
func chatModelDependencyMissing(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
