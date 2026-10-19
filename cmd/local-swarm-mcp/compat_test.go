package main

// Client-compatibility tests. MCP clients differ in which protocol version
// they speak and in how strictly they validate tool schemas before handing
// them to their model (several forward tools as OpenAI-style function
// definitions, which reject e.g. arrays without "items" or names over 64
// characters). These tests drive the binary with raw JSON-RPC over stdio -
// no mcp-go client - the way any third-party client would, and check every
// tool against the strictest of those rules.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// rawSession is a bare newline-delimited JSON-RPC stdio session.
type rawSession struct {
	t     *testing.T
	stdin io.WriteCloser
	lines chan []byte
}

func startRawStdio(t *testing.T, args ...string) *rawSession {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	s := &rawSession{t: t, stdin: stdin, lines: make(chan []byte, 16)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			s.lines <- append([]byte(nil), sc.Bytes()...)
		}
		close(s.lines)
	}()
	return s
}

func (s *rawSession) send(msg string) {
	s.t.Helper()
	if _, err := fmt.Fprintln(s.stdin, msg); err != nil {
		s.t.Fatalf("write: %v", err)
	}
}

// response waits for the JSON-RPC response with the given id.
func (s *rawSession) response(id int) map[string]any {
	s.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case line, ok := <-s.lines:
			if !ok {
				s.t.Fatal("server closed stdout")
			}
			var msg map[string]any
			if err := json.Unmarshal(line, &msg); err != nil {
				s.t.Fatalf("stdout carried a non-JSON-RPC line (breaks stdio clients): %q", line)
			}
			if got, ok := msg["id"].(float64); ok && int(got) == id {
				if e, ok := msg["error"]; ok {
					s.t.Fatalf("request %d failed: %v", id, e)
				}
				return msg["result"].(map[string]any)
			}
		case <-deadline:
			s.t.Fatalf("no response to request %d within 10s", id)
		}
	}
}

func TestCompat_NegotiatesEveryHandshakeProtocolVersion(t *testing.T) {
	// The versions clients in the wild still send in "initialize"
	// (Cursor, VS Code/Copilot, Codex, opencode, Claude Code, Gemini CLI
	// each pin their own).
	for _, version := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			s := startRawStdio(t, isolatedArgs(t.TempDir())...)
			s.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + version + `","capabilities":{},"clientInfo":{"name":"compat","version":"0"}}}`)
			init := s.response(1)
			if init["protocolVersion"] != version {
				t.Fatalf("negotiated protocolVersion = %v, want %s", init["protocolVersion"], version)
			}
			s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
			s.send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
			if tools, _ := s.response(2)["tools"].([]any); len(tools) == 0 {
				t.Fatal("tools/list returned no tools")
			}
		})
	}
}

// toolNamePattern is the strictest tool-name rule among common clients
// and model APIs (OpenAI-style function names).
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// longestClientPrefix is the longest prefix a client adds when exposing an
// MCP tool to its model ("mcp__<server>__<tool>" in Claude Code, with the
// README's server name); the result must still fit in 64 characters.
const longestClientPrefix = "mcp__local-swarm-mcp__"

func TestCompat_ToolSchemasSatisfyStrictClients(t *testing.T) {
	s := startRawStdio(t, append(isolatedArgs(t.TempDir()), "-allow-command-registration")...)
	s.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"compat","version":"0"}}}`)
	s.response(1)
	s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	s.send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools, _ := s.response(2)["tools"].([]any)

	const maxTools = 40 // the lowest per-session tool budget among popular clients
	if len(tools) > maxTools {
		t.Errorf("%d tools exceeds the %d some clients allow per session", len(tools), maxTools)
	}

	for _, raw := range tools {
		tool := raw.(map[string]any)
		name, _ := tool["name"].(string)
		if !toolNamePattern.MatchString(name) {
			t.Errorf("tool name %q doesn't match %s", name, toolNamePattern)
		}
		if n := len(longestClientPrefix + name); n > 64 {
			t.Errorf("tool %q is %d characters once prefixed as %s<tool>, over the 64 most model APIs allow", name, n, longestClientPrefix)
		}
		if desc, _ := tool["description"].(string); desc == "" {
			t.Errorf("tool %q has no description", name)
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("tool %q inputSchema.type = %v, want object", name, schema["type"])
		}
		checkSchema(t, name, "inputSchema", schema)
	}
}

// checkSchema walks a JSON schema, flagging constructs strict clients
// reject: arrays without "items", and properties without a "type".
func checkSchema(t *testing.T, tool, path string, schema map[string]any) {
	t.Helper()
	if schema["type"] == "array" {
		items, ok := schema["items"].(map[string]any)
		if !ok {
			t.Errorf("tool %q: %s is an array without \"items\"", tool, path)
		} else {
			checkSchema(t, tool, path+".items", items)
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for prop, raw := range props {
		p, _ := raw.(map[string]any)
		if _, ok := p["type"]; !ok {
			t.Errorf("tool %q: %s.%s has no \"type\"", tool, path, prop)
		}
		checkSchema(t, tool, path+"."+prop, p)
	}
}
