package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
)

const (
	DeepSeekFlash = "deepseek-flash"
	DeepSeekPro   = "deepseek-v4-pro"
)

// Edit the default model ID and endpoint here to configure DeepSeek.
const (
	deepSeekModelName = DeepSeekFlash
	deepSeekBaseURL   = "https://api.deepseek.com"
)

func newDeepSeek(ctx context.Context, modelName string) (model.ToolCallingChatModel, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = deepSeekModelName
	}
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required")
	}
	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  apiKey,
		Model:   modelName,
		BaseURL: deepSeekBaseURL,
		Timeout: time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return chatModel, nil
}
