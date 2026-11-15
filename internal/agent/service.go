package agent

import (
	"context"
	"errors"
	"fmt"
)

// Service coordinates agent validation and persistence through a shared repository.
// It can be reused between requests; callers own their mutable agent definitions.
type Service struct {
	repo *Repository
}

// NewService reuses a repository whose database pool is owned by the application.
func NewService(repo *Repository) (*Service, error) {
	if repo == nil || repo.db == nil {
		return nil, errors.New("agent: repository is required")
	}
	return &Service{repo: repo}, nil
}

// Save validates a and inserts def, assigning def.ID on success.
// The caller must pass the same definition that a.Validate checks.
// For a ChatModelAgent, pass a.Definition. Use Update for existing definitions.
func (s *Service) Save(ctx context.Context, a Agent, def *Definition) error {
	if err := s.validateForWrite(ctx, a, def); err != nil {
		return fmt.Errorf("agent: save: %w", err)
	}
	return s.repo.Save(ctx, def)
}

// Update validates a and updates its existing definition without changing ownership.
// The caller must pass the same definition that a.Validate checks.
func (s *Service) Update(ctx context.Context, a Agent, def *Definition) error {
	if err := s.validateForWrite(ctx, a, def); err != nil {
		return fmt.Errorf("agent: update: %w", err)
	}
	return s.repo.Update(ctx, def)
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
	if s == nil {
		return errors.New("agent: service is not initialized")
	}
	return s.repo.ready(ctx)
}

func (s *Service) validateForWrite(ctx context.Context, a Agent, def *Definition) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("%w: agent is required", ErrInvalidDefinition)
	}
	if err := validateDefinition(def); err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
	// Validation may take time. Check again before invoking persistence.
	return ctx.Err()
}
