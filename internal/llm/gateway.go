package llm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

// Gateway creates and reuses wrapped Eino clients for the configured models.
// Its model map is read-only after construction and can be shared by callers.
type Gateway struct {
	defaultModel string
	models       map[string]model.ToolCallingChatModel
}

// New loads .env and constructs a client for each model. Existing environment
// variables take precedence. Credentials are passed directly to the provider
// adapter and are never added to Config or ProviderConfig.
func New(ctx context.Context, cfg Config) (*Gateway, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("llm: load .env: %w", err)
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
			client, err := newDeepSeek(ctx, p, name)
			if err != nil {
				return nil, fmt.Errorf("llm: create model %q: %w", name, err)
			}
			g.models[name] = &gatewayModel{name: name, client: client}
		}
	}
	return g, nil
}

// Model returns a gateway-wrapped Eino model, including its WithTools method.
// An empty name uses default_model, or the first configured model if none is set.
func (g *Gateway) Model(name string) (model.ToolCallingChatModel, error) {
	if name == "" {
		name = g.defaultModel
	}
	chatModel, ok := g.models[name]
	if !ok {
		return nil, fmt.Errorf("llm: unsupported model %q", name)
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
