package service

import (
	"context"
	"errors"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

type AgentRepository interface {
	Save(context.Context, *model.Definition) error
	Update(context.Context, *model.Definition) error
	Delete(context.Context, uint64, uint64) error
	FindByID(context.Context, uint64, uint64) (*model.Definition, error)
	ListByUserID(context.Context, uint64, int, int) ([]model.Definition, error)
}

type DefinitionValidator interface {
	Validate(context.Context, *model.Definition) error
}

type AgentService struct {
	repo      AgentRepository
	validator DefinitionValidator
}

func NewAgentService(repo AgentRepository, validator DefinitionValidator) (*AgentService, error) {
	if repo == nil || validator == nil {
		return nil, errors.New("agent: repository and validator are required")
	}
	return &AgentService{repo: repo, validator: validator}, nil
}

func (s *AgentService) Save(ctx context.Context, def *model.Definition) error {
	if err := s.validator.Validate(ctx, def); err != nil {
		return err
	}
	return s.repo.Save(ctx, def)
}

func (s *AgentService) Update(ctx context.Context, def *model.Definition) error {
	if err := s.validator.Validate(ctx, def); err != nil {
		return err
	}
	return s.repo.Update(ctx, def)
}

func (s *AgentService) Delete(ctx context.Context, userID, id uint64) error {
	return s.repo.Delete(ctx, userID, id)
}
func (s *AgentService) FindByID(ctx context.Context, userID, id uint64) (*model.Definition, error) {
	return s.repo.FindByID(ctx, userID, id)
}
func (s *AgentService) ListByUserID(ctx context.Context, userID uint64, limit, offset int) ([]model.Definition, error) {
	return s.repo.ListByUserID(ctx, userID, limit, offset)
}
