package llm

import (
	"context"

	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// gatewayModel keeps calls through the gateway even when a caller uses the
// returned Eino model directly or derives a tool-bound model from it.
type gatewayModel struct {
	name   string
	client model.ToolCallingChatModel
}

func (m *gatewayModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.client.Generate(ctx, messages, opts...)
}

// Stream preserves the provider reader and its caller-owned close lifecycle.
func (m *gatewayModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.client.Stream(ctx, messages, opts...)
}

func (m *gatewayModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	client, err := m.client.WithTools(tools)
	if err != nil {
		return nil, err
	}
	bound := *m
	bound.client = client
	return &bound, nil
}

func (m *gatewayModel) GetType() string {
	if typ, ok := components.GetType(m.client); ok && typ != "" {
		return typ
	}
	return "Gateway"
}

// The provider owns callback instrumentation when it enables callbacks itself.
// Forwarding this flag prevents Eino from adding a second callback lifecycle.
func (m *gatewayModel) IsCallbacksEnabled() bool {
	return components.IsCallbacksEnabled(m.client)
}
