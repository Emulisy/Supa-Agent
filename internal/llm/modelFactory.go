package llm

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

type Provider string

const ProviderDeepSeek Provider = "deepseek"

// New returns an Eino chat model. An empty model name uses the provider's default.
// Add a case here when another provider is introduced.
func New(ctx context.Context, provider Provider, modelName string) (model.ToolCallingChatModel, error) {
	switch provider {
	case ProviderDeepSeek:
		return newDeepSeek(ctx, modelName)
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
