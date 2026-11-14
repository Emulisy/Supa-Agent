package gateway

import (
	"context"
	"fmt"

	"github.com/Emulisy/Supa-Agent/internal/llm/provider"
	"github.com/cloudwego/eino/components/model"
)

// Gateway holds a startup snapshot of logical models backed by reusable clients.
// Its maps are private and are never modified after New returns.
type Gateway struct {
	defaultModel string
	models       map[string]model.ToolCallingChatModel
}

func New(ctx context.Context, cfg Config, providers *provider.Registry) (*Gateway, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if providers == nil {
		return nil, fmt.Errorf("gateway: provider registry is required")
	}
	// Check every provider before constructing even the first client.
	for _, d := range cfg.Deployments {
		if !providers.Supports(d.Provider) {
			return nil, fmt.Errorf("gateway: deployment %q uses unsupported provider %q", d.ID, d.Provider)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	registry := NewRegistry()
	for _, d := range cfg.Deployments {
		client, err := providers.Create(ctx, provider.Config{
			Provider: d.Provider,
			Model:    d.Model,
			BaseURL:  d.BaseURL,
			APIKey:   d.APIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("gateway: initialize deployment %q: %w", d.ID, err)
		}
		if err := registry.Register(Deployment{
			ID:       d.ID,
			Provider: d.Provider,
			Model:    d.Model,
			Client:   client,
		}); err != nil {
			return nil, err
		}
	}

	g := &Gateway{
		defaultModel: cfg.DefaultModel,
		models:       make(map[string]model.ToolCallingChatModel, len(cfg.Models)),
	}
	for _, m := range cfg.Models {
		d, err := registry.Get(m.DeploymentIDs[0])
		if err != nil {
			return nil, err
		}
		g.models[m.Name] = &GatewayModel{name: m.Name, client: d.Client}
	}
	return g, nil
}

// Model returns an Eino-compatible model for a logical name. An empty name uses
// default_model, when configured. Physical model IDs are deployment settings.
func (g *Gateway) Model(name string) (model.ToolCallingChatModel, error) {
	if name == "" {
		name = g.defaultModel
	}
	m, ok := g.models[name]
	if !ok {
		return nil, fmt.Errorf("gateway: unknown logical model %q", name)
	}
	return m, nil
}
