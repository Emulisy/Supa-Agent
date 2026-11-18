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

	domain "github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

// ChatModelConfig is the JSON configuration stored in Definition.Config.
// An omitted model selects the current default at runtime. A zero MaxIterations
// uses Eino's default iteration limit.
type ChatModelConfig struct {
	domain.ModelConfig
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Instruction   string   `json:"instruction,omitempty"`
	MaxIterations int      `json:"max_iterations,omitempty"`
	Tools         []string `json:"tools,omitempty"`
}

// ChatModelAgent validates a persisted definition and builds an Eino agent.
// Persistence is coordinated by application services. Definition remains owned by
// the caller, so configuration is parsed again for every validation and build.
type ChatModelAgent struct {
	Definition *domain.Definition
}

// NewChatModelAgent validates configuration structure. Runtime checks the model
// against the platform catalog and supplies the resolved model when building.
func NewChatModelAgent(definition *domain.Definition) (*ChatModelAgent, error) {
	a := &ChatModelAgent{Definition: definition}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

// Validate checks the current definition and JSON configuration structure.
// It does not check model availability or invoke a provider.
func (a *ChatModelAgent) Validate() error {
	_, err := a.configuration()
	return err
}

// Build creates a fresh Eino ChatModelAgent using the current configuration and
// the model supplied by Runtime. It performs no model generation or persistence.
func (a *ChatModelAgent) Build(ctx context.Context, chatModel model.ToolCallingChatModel) (adk.Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	config, err := a.configuration()
	if err != nil {
		return nil, err
	}
	if chatModelDependencyMissing(chatModel) {
		return nil, errors.New("agent: chat model is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("agent: build chat model: %w", err)
	}
	adkConfig := &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   config.Description,
		Instruction:   config.Instruction,
		Model:         chatModel,
		MaxIterations: config.MaxIterations,
	}
	for _, name := range config.Tools {
		t, err := builtinTool(name)
		if err != nil {
			return nil, fmt.Errorf("agent: build tool: %w", err)
		}
		adkConfig.ToolsConfig.Tools = append(adkConfig.ToolsConfig.Tools, t)
	}
	built, err := adk.NewChatModelAgent(ctx, adkConfig)
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
		return nil, fmt.Errorf("%w: chat model agent is not initialized", domain.ErrInvalidDefinition)
	}
	if err := domain.ValidateDefinition(a.Definition); err != nil {
		return nil, err
	}
	if a.Definition.Type != domain.TypeChatModel {
		return nil, fmt.Errorf("%w: chat model agent requires type %q", domain.ErrInvalidDefinition, domain.TypeChatModel)
	}
	decoder := json.NewDecoder(bytes.NewReader(a.Definition.Config))
	decoder.DisallowUnknownFields()
	var config *ChatModelConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%w: decode chat model config: %v", domain.ErrInvalidDefinition, err)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: chat model config must be an object", domain.ErrInvalidDefinition)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: chat model config must contain one JSON object", domain.ErrInvalidDefinition)
	}
	if strings.TrimSpace(config.Name) == "" {
		return nil, fmt.Errorf("%w: chat model name is required", domain.ErrInvalidDefinition)
	}
	if config.Model != "" && strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("%w: chat model selector must not be whitespace", domain.ErrInvalidDefinition)
	}
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("%w: max_iterations must be non-negative", domain.ErrInvalidDefinition)
	}
	seen := make(map[string]bool, len(config.Tools))
	for _, name := range config.Tools {
		if name != "calculator" && name != "datetime" {
			return nil, fmt.Errorf("%w: unknown tool %q", domain.ErrInvalidDefinition, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: duplicate tool %q", domain.ErrInvalidDefinition, name)
		}
		seen[name] = true
	}
	return config, nil
}

// An Eino model interface may contain a nil pointer.
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
