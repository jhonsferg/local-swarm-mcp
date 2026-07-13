// Package config loads local-swarm-mcp's YAML configuration: the list of
// OpenAI-compatible inference backends and the scratch-store location.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Backend describes a single OpenAI-compatible inference endpoint, whether
// that's a local llama.cpp/Ollama server or a remote provider.
type Backend struct {
	Name    string `yaml:"name" json:"name"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model   string `yaml:"model" json:"model"`
}

// MCPServer describes a downstream MCP server whose tools can be made
// available to tool-using agent tasks (see spawn_agent_task), spawned as a
// local stdio subprocess the same way a client like Claude Code registers
// one in its own .mcp.json.
type MCPServer struct {
	Name    string   `yaml:"name" json:"name"`
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
}

// Config is the top-level local-swarm-mcp configuration.
type Config struct {
	Backends   []Backend   `yaml:"backends" json:"backends"`
	MCPServers []MCPServer `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
	StorePath  string      `yaml:"store_path,omitempty" json:"store_path,omitempty"`
}

// Load reads and parses a config file from path, auto-detecting YAML vs
// JSON from its extension (.json => JSON, anything else => YAML), and
// filling in defaults for any fields left unset. Pass a non-empty format
// ("yaml" or "json") to override auto-detection.
func Load(path, format string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg, err := parse(data, resolveFormat(path, format))
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if cfg.StorePath == "" {
		cfg.StorePath = DefaultStorePath()
	}

	return cfg, nil
}

func resolveFormat(path, format string) string {
	if format != "" {
		return format
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "json"
	}
	return "yaml"
}

func parse(data []byte, format string) (*Config, error) {
	var cfg Config
	var err error
	switch format {
	case "json":
		err = json.Unmarshal(data, &cfg)
	case "yaml":
		err = yaml.Unmarshal(data, &cfg)
	default:
		return nil, fmt.Errorf("unknown config format %q (want \"yaml\" or \"json\")", format)
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultStorePath returns the scratch-store path used when none is set in
// config or on the command line.
func DefaultStorePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp-scratch.db"
	}
	return filepath.Join(dir, "local-swarm-mcp", "scratch.db")
}
