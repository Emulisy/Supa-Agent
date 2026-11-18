package service

import (
	"context"
	"errors"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

type MemoryRepository interface {
	List(context.Context, model.MemoryScope, bool, int, int) ([]model.Memory, error)
	Archive(context.Context, model.MemoryScope, uint64) error
	Delete(context.Context, model.MemoryScope, uint64) error
}

type MemoryService struct {
	agents AgentLookup
	repo   MemoryRepository
}

func NewMemoryService(agents AgentLookup, repo MemoryRepository) (*MemoryService, error) {
	if agents == nil || repo == nil {
		return nil, errors.New("memory: all dependencies are required")
	}
	return &MemoryService{agents: agents, repo: repo}, nil
}

func (s *MemoryService) List(ctx context.Context, scope model.MemoryScope, limit, offset int) ([]model.Memory, error) {
	if limit < 1 || limit > model.MaxPageSize || offset < 0 {
		return nil, model.ErrInvalidPagination
	}
	if _, err := s.agents.FindByID(ctx, scope.UserID, scope.AgentID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, scope, false, limit, offset)
}

func (s *MemoryService) Archive(ctx context.Context, scope model.MemoryScope, id uint64) error {
	return s.repo.Archive(ctx, scope, id)
}

func (s *MemoryService) Delete(ctx context.Context, scope model.MemoryScope, id uint64) error {
	return s.repo.Delete(ctx, scope, id)
}
