package llm

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// gatewayModel keeps calls through the gateway even when a caller uses the
// returned Eino model directly or derives a tool-bound model from it.
// Provider failures return the package errors without switching models.
type gatewayModel struct {
	client model.ToolCallingChatModel
}

func (m *gatewayModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	message, err := m.client.Generate(ctx, messages, opts...)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return message, ErrGenerateFailed
	}
	return message, err
}

// Stream preserves the provider reader and its caller-owned close lifecycle.
// Errors received after streaming starts propagate directly from that reader.
func (m *gatewayModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := m.client.Stream(ctx, messages, opts...)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return reader, ErrStreamFailed
	}
	return reader, err
}

func (m *gatewayModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	client, err := m.client.WithTools(tools)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrBindToolsFailed
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
