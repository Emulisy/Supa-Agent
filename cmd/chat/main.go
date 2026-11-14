package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/Emulisy/Supa-Agent/internal/llm/gateway"
	"github.com/Emulisy/Supa-Agent/internal/llm/provider"
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
	configPath := flags.String("config", "configs/llm.yaml", "LLM deployment and logical model configuration")
	modelName := flags.String("model", "", "Logical model name (empty uses default_model from YAML)")
	timeout := flags.Duration("timeout", 0, "Overall request deadline, including streaming (zero uses provider defaults)")
	streaming := flags.Bool("stream", false, "Stream the response")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	// A missing .env is fine: environment variables can also come from the shell.
	// Existing shell variables take precedence over values loaded from .env.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg, err := gateway.LoadConfigFile(*configPath)
	if err != nil {
		return err
	}
	providers := provider.NewRegistry()
	if err := providers.Register(provider.DeepSeek, provider.NewDeepSeek); err != nil {
		return err
	}
	chatGateway, err := gateway.New(ctx, cfg, providers)
	if err != nil {
		return err
	}
	chatModel, err := chatGateway.Model(*modelName)
	if err != nil {
		return err
	}

	prompt := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if prompt == "" {
		prompt = "Explain what an AI agent is in three sentences."
	}
	input := []*schema.Message{
		schema.SystemMessage("You are a helpful assistant. Keep answers concise."),
		schema.UserMessage(prompt),
	}
	if *timeout > 0 {
		var cancelRequest context.CancelFunc
		ctx, cancelRequest = context.WithTimeout(ctx, *timeout)
		defer cancelRequest()
	}
	if *streaming {
		reader, err := chatModel.Stream(ctx, input)
		if err != nil {
			return fmt.Errorf("stream response: %w", err)
		}
		defer reader.Close()
		for {
			chunk, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				fmt.Println()
				return nil
			}
			if err != nil {
				return fmt.Errorf("receive response: %w", err)
			}
			fmt.Print(chunk.Content)
		}
	}
	response, err := chatModel.Generate(ctx, input)
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}
	fmt.Println(response.Content)
	return nil
}
