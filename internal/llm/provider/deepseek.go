package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
)

const DeepSeek = "deepseek"

// NewDeepSeek initializes the native Eino DeepSeek adapter without making a
// completion request. Generate, Stream and WithTools are provided by the adapter.
func NewDeepSeek(ctx context.Context, cfg Config) (model.ToolCallingChatModel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("deepseek: model is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("deepseek: API key is required")
	}
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
	})
}
