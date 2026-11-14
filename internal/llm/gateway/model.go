package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// GatewayModel implements Eino's model interface for one logical model.
type GatewayModel struct {
	name   string
	client model.ToolCallingChatModel
}

var _ model.ToolCallingChatModel = (*GatewayModel)(nil)

func (m *GatewayModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	response, err := m.client.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("gateway: generate with logical model %q: %w", m.name, err)
	}
	return response, nil
}

// Stream passes through the provider reader. The caller must close it.
func (m *GatewayModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := m.client.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("gateway: stream with logical model %q: %w", m.name, err)
	}
	return reader, nil
}

// WithTools derives a new model without changing the shared client or wrapper.
func (m *GatewayModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	// Validate descriptors before handing them to the native adapter, which
	// expects non-nil tool pointers and unique function names.
	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("gateway: bind tools for logical model %q: each tool requires a name", m.name)
		}
		if _, exists := names[tool.Name]; exists {
			return nil, fmt.Errorf("gateway: bind tools for logical model %q: duplicate tool %q", m.name, tool.Name)
		}
		names[tool.Name] = struct{}{}
	}
	bound, err := m.client.WithTools(tools)
	if err != nil {
		return nil, fmt.Errorf("gateway: bind tools for logical model %q: %w", m.name, err)
	}
	return &GatewayModel{name: m.name, client: bound}, nil
}
