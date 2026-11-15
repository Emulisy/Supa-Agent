package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/cloudwego/eino/adk"
)

// Service coordinates agent validation, runtime construction, and persistence.
// Share it between requests; callers own their definitions.
type Service struct {
	repo    *Repository
	gateway *llm.Gateway
}

// NewService reuses the application's repository and initialized model gateway.
// The application owns the database pool and gateway lifecycle.
func NewService(repo *Repository, gateway *llm.Gateway) (*Service, error) {
	if repo == nil || repo.db == nil {
		return nil, errors.New("agent: repository is required")
	}
	if gateway == nil {
		return nil, errors.New("agent: model gateway is required")
	}
	return &Service{repo: repo, gateway: gateway}, nil
}

// Save validates and inserts def, assigning its generated ID on success.
// An omitted model remains omitted and is selected at runtime.
func (s *Service) Save(ctx context.Context, def *Definition) error {
	if _, err := s.validateAgent(ctx, def); err != nil {
		return fmt.Errorf("agent: save: %w", err)
	}
	return s.repo.Save(ctx, def)
}

// Update validates and replaces an owned definition without filling an omitted
// model. The gateway selects the current default when the agent is built.
func (s *Service) Update(ctx context.Context, def *Definition) error {
	if _, err := s.validateAgent(ctx, def); err != nil {
		return fmt.Errorf("agent: update: %w", err)
	}
	return s.repo.Update(ctx, def)
}

// Build validates def, resolves its configured model, and constructs a runtime.
// The gateway selects the current default when no model is given. Build does
// not change def, persist anything, or invoke model generation.
func (s *Service) Build(ctx context.Context, def *Definition) (adk.Agent, error) {
	a, err := s.validateAgent(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("agent: build: %w", err)
	}
	var config ModelConfig
	if err := json.Unmarshal(def.Config, &config); err != nil {
		return nil, fmt.Errorf("%w: decode model config: %v", ErrInvalidDefinition, err)
	}
	chatModel, err := s.gateway.Model(config.Model)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve model: %w", err)
	}
	return a.Build(ctx, chatModel)
}

// Delete removes the authenticated user's definition. It does not validate config,
// so invalid or obsolete agent definitions can still be removed.
func (s *Service) Delete(ctx context.Context, userID, id uint64) error {
	if err := s.ready(ctx); err != nil {
		return fmt.Errorf("agent: delete: %w", err)
	}
	return s.repo.Delete(ctx, userID, id)
}

// FindByID loads a definition belonging to the authenticated userID.
func (s *Service) FindByID(ctx context.Context, userID, id uint64) (*Definition, error) {
	if err := s.ready(ctx); err != nil {
		return nil, fmt.Errorf("agent: find by ID: %w", err)
	}
	return s.repo.FindByID(ctx, userID, id)
}

// ListByUserID lists the authenticated owner's definitions using repository pagination.
func (s *Service) ListByUserID(ctx context.Context, userID uint64, limit, offset int) ([]Definition, error) {
	if err := s.ready(ctx); err != nil {
		return nil, fmt.Errorf("agent: list: %w", err)
	}
	return s.repo.ListByUserID(ctx, userID, limit, offset)
}

func (s *Service) ready(ctx context.Context) error {
	if s == nil || s.gateway == nil {
		return errors.New("agent: service is not initialized")
	}
	return s.repo.ready(ctx)
}

// validateAgent checks configuration and catalog membership without resolving
// a model or filling the default.
func (s *Service) validateAgent(ctx context.Context, def *Definition) (Agent, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	if err := validateDefinition(def); err != nil {
		return nil, err
	}
	var a Agent
	switch def.Type {
	case TypeChatModel:
		var err error
		a, err = NewChatModelAgent(def)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unsupported agent type %q", ErrInvalidDefinition, def.Type)
	}
	var config ModelConfig
	if err := json.Unmarshal(def.Config, &config); err != nil {
		return nil, fmt.Errorf("%w: decode model config: %v", ErrInvalidDefinition, err)
	}
	if err := s.gateway.ValidateModel(config.Model); err != nil {
		return nil, fmt.Errorf("agent: validate model: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a, nil
}
