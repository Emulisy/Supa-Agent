package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
)

const ProviderDeepSeek = "deepseek"

func newDeepSeek(ctx context.Context, cfg ProviderConfig, name, apiKey string) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("deepseek: DEEPSEEK_API_KEY is required in .env or the process environment")
	}
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		Model:   name,
		BaseURL: cfg.BaseURL,
		APIKey:  apiKey,
	})
}
