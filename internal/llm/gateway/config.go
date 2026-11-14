// Package gateway maps logical model names to configured Eino model clients.
package gateway

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeploymentConfig defines one physical model, endpoint and credential reference.
type DeploymentConfig struct {
	ID        string `yaml:"id"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url,omitempty"`
	APIKeyEnv string `yaml:"api_key_env"`
	// APIKey is resolved by the loader, or supplied directly by Go callers.
	// YAML contains only the environment variable name, never the credential.
	APIKey string `yaml:"-" json:"-"`
}

// ModelConfig gives an application-facing name to a deployment. The slice keeps
// the configuration ready for routing; Phase 1 requires exactly one deployment.
type ModelConfig struct {
	Name          string   `yaml:"name"`
	DeploymentIDs []string `yaml:"deployment_ids"`
}

type Config struct {
	DefaultModel string             `yaml:"default_model,omitempty"`
	Deployments  []DeploymentConfig `yaml:"deployments"`
	Models       []ModelConfig      `yaml:"models"`
}

// Validate checks the complete resolved configuration. New additionally checks
// that all providers are registered before initializing any clients.
func (c Config) Validate() error {
	if len(c.Deployments) == 0 {
		return fmt.Errorf("gateway: at least one deployment is required")
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("gateway: at least one logical model is required")
	}
	deployments := make(map[string]struct{}, len(c.Deployments))
	for i, d := range c.Deployments {
		if strings.TrimSpace(d.ID) == "" {
			return fmt.Errorf("gateway: deployment at index %d has an empty ID", i)
		}
		if _, exists := deployments[d.ID]; exists {
			return fmt.Errorf("gateway: duplicate deployment %q", d.ID)
		}
		deployments[d.ID] = struct{}{}
		if strings.TrimSpace(d.Provider) == "" || strings.TrimSpace(d.Model) == "" {
			return fmt.Errorf("gateway: deployment %q requires provider and model", d.ID)
		}
		if strings.TrimSpace(d.APIKey) == "" {
			return fmt.Errorf("gateway: deployment %q requires a resolved API key", d.ID)
		}
		if d.BaseURL != "" {
			u, err := url.Parse(d.BaseURL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("gateway: deployment %q requires an HTTP(S) base URL without credentials, query or fragment", d.ID)
			}
		}
	}
	models := make(map[string]struct{}, len(c.Models))
	for i, m := range c.Models {
		if strings.TrimSpace(m.Name) == "" {
			return fmt.Errorf("gateway: logical model at index %d has an empty name", i)
		}
		if _, exists := models[m.Name]; exists {
			return fmt.Errorf("gateway: duplicate logical model %q", m.Name)
		}
		models[m.Name] = struct{}{}
		if len(m.DeploymentIDs) != 1 {
			return fmt.Errorf("gateway: logical model %q must reference exactly one deployment in Phase 1", m.Name)
		}
		if _, exists := deployments[m.DeploymentIDs[0]]; !exists {
			return fmt.Errorf("gateway: logical model %q references unknown deployment %q", m.Name, m.DeploymentIDs[0])
		}
	}
	if c.DefaultModel != "" {
		if _, exists := models[c.DefaultModel]; !exists {
			return fmt.Errorf("gateway: default model %q is not configured", c.DefaultModel)
		}
	}
	return nil
}

// LoadConfigFile reads one YAML document and resolves credential references from
// the process environment. Callers may load a .env file before calling it.
func LoadConfigFile(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("gateway: open config: %w", err)
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("gateway: decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, fmt.Errorf("gateway: decode trailing config: %w", err)
		}
		return Config{}, fmt.Errorf("gateway: config must contain exactly one YAML document")
	}
	for i := range cfg.Deployments {
		d := &cfg.Deployments[i]
		if strings.TrimSpace(d.APIKeyEnv) == "" {
			return Config{}, fmt.Errorf("gateway: deployment %q requires api_key_env", d.ID)
		}
		key := os.Getenv(d.APIKeyEnv)
		if strings.TrimSpace(key) == "" {
			return Config{}, fmt.Errorf("gateway: deployment %q requires nonempty environment variable %q", d.ID, d.APIKeyEnv)
		}
		d.APIKey = key
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
