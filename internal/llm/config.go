// Package llm loads provider definitions and provides access to Eino chat models.
package llm

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig groups shared provider settings and its supported model IDs.
// Credentials are loaded separately from the environment.
type ProviderConfig struct {
	Name            string   `yaml:"name"`
	BaseURL         string   `yaml:"base_url,omitempty"`
	AvailableModels []string `yaml:"available_models"`
}

type Config struct {
	DefaultModel string           `yaml:"default_model,omitempty"`
	Providers    []ProviderConfig `yaml:"providers"`
}

func (c Config) Validate() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("llm: at least one provider is required")
	}
	names := make(map[string]struct{})
	for i, p := range c.Providers {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("llm: provider at index %d has an empty name", i)
		}
		if p.Name != ProviderDeepSeek {
			return fmt.Errorf("llm: unsupported provider %q", p.Name)
		}
		if p.BaseURL != "" {
			u, err := url.Parse(p.BaseURL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("llm: provider %q requires an HTTP(S) base URL without credentials, query or fragment", p.Name)
			}
		}
		if len(p.AvailableModels) == 0 {
			return fmt.Errorf("llm: provider %q requires at least one available model", p.Name)
		}
		for j, name := range p.AvailableModels {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("llm: provider %q model at index %d has an empty name", p.Name, j)
			}
			if _, exists := names[name]; exists {
				return fmt.Errorf("llm: duplicate model %q", name)
			}
			names[name] = struct{}{}
		}
	}
	if c.DefaultModel != "" {
		if _, exists := names[c.DefaultModel]; !exists {
			return fmt.Errorf("llm: default model %q is not configured", c.DefaultModel)
		}
	}
	return nil
}

// LoadConfigFile reads provider settings and available models. It does not load .env,
// resolve credentials, or create clients, so it also works without API keys.
func LoadConfigFile(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("llm: open config: %w", err)
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("llm: decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, fmt.Errorf("llm: decode trailing config: %w", err)
		}
		return Config{}, fmt.Errorf("llm: config must contain exactly one YAML document")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
