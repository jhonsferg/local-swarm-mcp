package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "backends:\n  - name: local\n    base_url: http://localhost:8080/v1\n    model: qwen\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Backends) != 1 || cfg.Backends[0].Name != "local" {
		t.Fatalf("unexpected backends: %+v", cfg.Backends)
	}
	if cfg.StorePath == "" {
		t.Fatal("expected default StorePath to be set")
	}
}

func TestLoad_MCPServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "backends:\n  - name: local\n    base_url: http://localhost:8080/v1\n    model: qwen\n" +
		"mcp_servers:\n  - name: codebase-memory-mcp\n    command: /path/to/codebase-memory-mcp\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 mcp_server, got %d", len(cfg.MCPServers))
	}
	if cfg.MCPServers[0].Name != "codebase-memory-mcp" || cfg.MCPServers[0].Command != "/path/to/codebase-memory-mcp" {
		t.Fatalf("unexpected mcp_server: %+v", cfg.MCPServers[0])
	}
}

func TestLoad_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"backends":[{"name":"local","base_url":"http://localhost:8080/v1","model":"qwen"}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Backends) != 1 || cfg.Backends[0].Model != "qwen" {
		t.Fatalf("unexpected backends: %+v", cfg.Backends)
	}
}

func TestLoad_ExplicitFormatOverride(t *testing.T) {
	dir := t.TempDir()
	// .yaml extension, but content is JSON - explicit override should still parse it.
	path := filepath.Join(dir, "config.yaml")
	content := `{"backends":[{"name":"x","base_url":"http://x","model":"y"}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path, "json"); err != nil {
		t.Fatalf("Load with json override: %v", err)
	}
}

func TestLoad_UnknownFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("backends: []"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path, "toml"); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), ""); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
