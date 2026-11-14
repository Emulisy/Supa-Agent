// Package provider constructs Eino model clients from deployment settings.
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// Config contains the resolved settings for a provider client.
type Config struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
}

// Factory initializes one reusable model client.
type Factory func(context.Context, Config) (model.ToolCallingChatModel, error)

// Registry holds provider factories. Register them at startup, before creating
// the gateway, and do not modify the registry while it is in use.
type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(name string, factory Factory) error {
	if strings.TrimSpace(name) == "" || factory == nil {
		return fmt.Errorf("provider: name and factory are required")
	}
	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("provider: duplicate registration %q", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Supports(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.factories[name]
	return ok
}

func (r *Registry) Create(ctx context.Context, cfg Config) (model.ToolCallingChatModel, error) {
	if !r.Supports(cfg.Provider) {
		return nil, fmt.Errorf("provider: unsupported provider %q", cfg.Provider)
	}
	client, err := r.factories[cfg.Provider](ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("provider %q: create model: %w", cfg.Provider, err)
	}
	if client == nil {
		return nil, fmt.Errorf("provider %q: factory returned a nil model", cfg.Provider)
	}
	return client, nil
}
