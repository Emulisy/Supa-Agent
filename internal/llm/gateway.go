package llm

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Gateway creates and reuses wrapped Eino clients for the configured models.
// Its model map is read-only after construction and can be shared by callers.
// Model failures are returned to the caller; they never select another model.
type Gateway struct {
	defaultModel string
	models       map[string]model.ToolCallingChatModel
}

// New constructs reusable clients with explicitly supplied provider credentials.
// Credentials are never retained in Config or ProviderConfig.
func New(ctx context.Context, cfg Config, credentials map[string]string) (*Gateway, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	g := &Gateway{
		defaultModel: cfg.DefaultModel,
		models:       make(map[string]model.ToolCallingChatModel),
	}
	if g.defaultModel == "" {
		g.defaultModel = cfg.Providers[0].AvailableModels[0]
	}
	for _, p := range cfg.Providers {
		for _, name := range p.AvailableModels {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Config validation currently allows only the DeepSeek provider.
			client, err := newDeepSeek(ctx, p, name, credentials[p.Name])
			if err != nil {
				return nil, fmt.Errorf("llm: create model %q: %w", name, err)
			}
			g.models[name] = &gatewayModel{client: client}
		}
	}
	return g, nil
}

// ValidateModel checks an explicit model ID against the configured catalog
// without resolving a client. An empty name defers selection to runtime.
func (g *Gateway) ValidateModel(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := g.models[name]; !ok {
		return ErrUnsupportedModel
	}
	return nil
}

// Model returns a gateway-wrapped Eino model, including its WithTools method.
// An empty name uses default_model, or the first configured model if none is set.
// This default selection is not a fallback for failures of an explicit model.
func (g *Gateway) Model(name string) (model.ToolCallingChatModel, error) {
	if name == "" {
		name = g.defaultModel
	}
	chatModel, ok := g.models[name]
	if !ok {
		return nil, ErrUnsupportedModel
	}
	return chatModel, nil
}

func (g *Gateway) Generate(ctx context.Context, name string, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	chatModel, err := g.Model(name)
	if err != nil {
		return nil, err
	}
	return chatModel.Generate(ctx, messages, opts...)
}

// Stream delegates to Eino. The caller must close the returned reader.
func (g *Gateway) Stream(ctx context.Context, name string, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	chatModel, err := g.Model(name)
	if err != nil {
		return nil, err
	}
	return chatModel.Stream(ctx, messages, opts...)
}
