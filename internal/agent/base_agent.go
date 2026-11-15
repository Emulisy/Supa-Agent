package agent

import (
	"context"
	"errors"
	"fmt"
)

// BaseAgent provides shared persistence for concrete agents that embed it.
// Concrete agents implement Validate and Build, and pass their Validate method
// to NewBaseAgent. Each instance holds a mutable, caller-owned Definition.
type BaseAgent struct {
	Definition *Definition
	repo       *Repository
	validate   func() error
}

// NewBaseAgent checks the dependencies without invoking validate, so a concrete
// agent can finish assigning the returned base before its validator is called.
// The validator must check agent-specific configuration without persisting it.
func NewBaseAgent(definition *Definition, repo *Repository, validate func() error) (*BaseAgent, error) {
	if definition == nil {
		return nil, fmt.Errorf("%w: definition is required", ErrInvalidDefinition)
	}
	if repo == nil || repo.db == nil {
		return nil, errors.New("agent: repository is required")
	}
	if validate == nil {
		return nil, errors.New("agent: validator is required")
	}
	return &BaseAgent{
		Definition: definition,
		repo:       repo,
		validate:   validate,
	}, nil
}

// Save validates a new definition and persists it through Repository.Save.
// On success, Definition.ID contains the database-generated ID.
func (a *BaseAgent) Save(ctx context.Context) error {
	if err := a.validateForWrite(ctx); err != nil {
		return fmt.Errorf("agent: save: %w", err)
	}
	return a.repo.Save(ctx, a.Definition)
}

// Update validates and persists changes to an existing owned definition.
func (a *BaseAgent) Update(ctx context.Context) error {
	if err := a.validateForWrite(ctx); err != nil {
		return fmt.Errorf("agent: update: %w", err)
	}
	return a.repo.Update(ctx, a.Definition)
}

// Delete removes the definition belonging to Definition.UserID. Configuration
// validation is skipped so an agent with invalid configuration can be removed.
func (a *BaseAgent) Delete(ctx context.Context) error {
	if err := a.ready(ctx); err != nil {
		return fmt.Errorf("agent: delete: %w", err)
	}
	return a.repo.Delete(ctx, a.Definition.UserID, a.Definition.ID)
}

func (a *BaseAgent) ready(ctx context.Context) error {
	if a == nil {
		return errors.New("agent: base agent is not initialized")
	}
	if err := a.repo.ready(ctx); err != nil {
		return err
	}
	if a.Definition == nil {
		return fmt.Errorf("%w: definition is required", ErrInvalidDefinition)
	}
	return nil
}

func (a *BaseAgent) validateForWrite(ctx context.Context) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	if a.validate == nil {
		return errors.New("agent: validator is not configured")
	}
	if err := validateDefinition(a.Definition); err != nil {
		return err
	}
	if err := a.validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	// A validator may take time; honor cancellation before invoking persistence.
	return ctx.Err()
}
