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

	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/cloudwego/eino/schema"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	configPath := flags.String("config", "configs/llm.yaml", "Supported model configuration")
	modelName := flags.String("model", "", "Configured model ID (empty uses default_model from YAML)")
	listModels := flags.Bool("list-models", false, "List configured model IDs without loading credentials")
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

	cfg, err := llm.LoadConfigFile(*configPath)
	if err != nil {
		return err
	}
	if *listModels {
		for _, p := range cfg.Providers {
			for _, name := range p.AvailableModels {
				fmt.Println(name)
			}
		}
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	chatGateway, err := llm.New(ctx, cfg)
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
		reader, err := chatGateway.Stream(ctx, *modelName, input)
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
	response, err := chatGateway.Generate(ctx, *modelName, input)
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}
	fmt.Println(response.Content)
	return nil
}
