// Package skills holds the Agent Skills (https://agentskills.io) shipped
// with local-swarm-mcp. This test keeps each one within the open SKILL.md
// specification, so the same folder loads in every agent that implements
// it (Claude Code, Codex, Cursor, GitHub Copilot, opencode, Gemini CLI).
package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	// nameRule: 1-64 lowercase letters/digits, single hyphens between them.
	nameRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// markdownLink captures the target of [text](target).
	markdownLink = regexp.MustCompile(`\]\(([^)]+)\)`)
)

// allowedKeys are the frontmatter fields the specification defines.
var allowedKeys = map[string]bool{
	"name": true, "description": true, "license": true,
	"compatibility": true, "metadata": true, "allowed-tools": true,
}

func TestSkillsFollowAgentSkillsSpec(t *testing.T) {
	dirs, err := filepath.Glob("*/SKILL.md")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no skills found (%v)", err)
	}
	for _, path := range dirs {
		dir := filepath.Dir(path)
		t.Run(dir, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			if !strings.HasPrefix(text, "---\n") {
				t.Fatal("SKILL.md must start with YAML frontmatter")
			}
			front, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
			if !ok {
				t.Fatal("unterminated frontmatter")
			}

			var meta map[string]any
			if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
				t.Fatalf("frontmatter is not valid YAML: %v", err)
			}
			for key := range meta {
				if !allowedKeys[key] {
					t.Errorf("frontmatter key %q is not in the specification", key)
				}
			}

			name, _ := meta["name"].(string)
			if len(name) > 64 || !nameRule.MatchString(name) {
				t.Errorf("name %q must be 1-64 lowercase letters, digits, and single hyphens", name)
			}
			if name != dir {
				t.Errorf("name %q must match its directory %q", name, dir)
			}
			if desc, _ := meta["description"].(string); len(desc) == 0 || len(desc) > 1024 {
				t.Errorf("description must be 1-1024 characters, got %d", len(desc))
			}
			if compat, ok := meta["compatibility"].(string); ok && len(compat) > 500 {
				t.Errorf("compatibility must be at most 500 characters, got %d", len(compat))
			}

			for _, m := range markdownLink.FindAllStringSubmatch(body, -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
					continue
				}
				if _, err := os.Stat(filepath.Join(dir, target)); err != nil {
					t.Errorf("relative link %q points at a missing file", target)
				}
			}
		})
	}
}
