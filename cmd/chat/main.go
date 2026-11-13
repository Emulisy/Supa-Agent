package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	modelName := flags.String("model", "", "DeepSeek model ID (empty uses the provider's default)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// A missing .env is fine: environment variables can also come from the shell.
	// Existing shell variables take precedence over values loaded from .env.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	chatModel, err := llm.New(ctx, llm.ProviderDeepSeek, *modelName)
	if err != nil {
		return err
	}

	prompt := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if prompt == "" {
		prompt = "Explain what an AI agent is in three sentences."
	}
	response, err := chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage("You are a helpful assistant. Keep answers concise."),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}
	fmt.Println(response.Content)
	return nil
}
