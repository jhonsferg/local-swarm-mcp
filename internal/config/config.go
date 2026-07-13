// Package config loads local-swarm-mcp's YAML configuration: the list of
// OpenAI-compatible inference backends and the scratch-store location.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Backend describes a single OpenAI-compatible inference endpoint, whether
// that's a local llama.cpp/Ollama server or a remote provider.
type Backend struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key,omitempty"`
	Model   string `yaml:"model"`
}

// Config is the top-level local-swarm-mcp configuration.
type Config struct {
	Backends  []Backend `yaml:"backends"`
	StorePath string    `yaml:"store_path"`
}

// Load reads and parses a YAML config file from path, filling in defaults
// for any fields left unset.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if cfg.StorePath == "" {
		cfg.StorePath = defaultStorePath()
	}

	return &cfg, nil
}

func defaultStorePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp-scratch.db"
	}
	return filepath.Join(dir, "local-swarm-mcp", "scratch.db")
}
