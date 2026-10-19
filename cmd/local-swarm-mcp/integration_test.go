package main

// End-to-end tests: each one builds the real binary once (TestMain), runs
// it as an MCP client or operator would - over stdio through mcp-go's own
// client, or as an HTTP daemon - against fake inference backends, with
// every store, lock, log, and config path isolated in a temp dir so a
// test never touches the user's real local-swarm-mcp state.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	bolt "go.etcd.io/bbolt"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lsm-integration-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdir temp:", err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "local-swarm-mcp")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build binary:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// isolatedArgs points every on-disk path the binary uses into dir.
func isolatedArgs(dir string) []string {
	return []string{
		"-config", filepath.Join(dir, "no-such-config.yaml"),
		"-store-path", filepath.Join(dir, "scratch.db"),
		"-host-store-path", filepath.Join(dir, "hosts.db"),
		"-mcp-server-store-path", filepath.Join(dir, "mcp-servers.db"),
		"-log-path", filepath.Join(dir, "daemon.log"),
		"-lock-path", filepath.Join(dir, "daemon.lock"),
	}
}

// fakeBackend is an OpenAI-compatible (and Ollama /api/tags) fake that
// records the last chat request body it received.
type fakeBackend struct {
	*httptest.Server
	mu   sync.Mutex
	last map[string]any
}

func newFakeBackend(t *testing.T, reply string) *fakeBackend {
	t.Helper()
	f := &fakeBackend{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fake:1b"}]}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"fake:1b"}]}`))
		case "/v1/chat/completions":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.last = body
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeBackend) lastRequest() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// startStdio spawns the binary as an MCP stdio server and completes the
// initialize handshake, failing the test if that takes longer than 10s.
func startStdio(t *testing.T, args ...string) *mcpclient.Client {
	t.Helper()
	c, err := mcpclient.NewStdioMCPClient(binPath, nil, args...)
	if err != nil {
		t.Fatalf("start stdio server: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "integration-test", Version: "0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

func callTool(t *testing.T, c *mcpclient.Client, name string, args map[string]any) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var text strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

func toolNames(t *testing.T, c *mcpclient.Client) map[string]bool {
	t.Helper()
	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
}

func TestStdio_ZeroBackendsServesToolsAndGatesCommandRegistration(t *testing.T) {
	c := startStdio(t, isolatedArgs(t.TempDir())...)
	names := toolNames(t, c)
	for _, want := range []string{"delegate_task", "spawn_task", "scratch_set", "register_backend_host", "list_downstream_mcp_servers"} {
		if !names[want] {
			t.Errorf("tool %q missing from %v", want, names)
		}
	}
	if names["register_downstream_mcp_server"] {
		t.Error("register_downstream_mcp_server must not be offered without -allow-command-registration")
	}

	allowed := startStdio(t, append(isolatedArgs(t.TempDir()), "-allow-command-registration")...)
	if !toolNames(t, allowed)["register_downstream_mcp_server"] {
		t.Error("register_downstream_mcp_server should be offered with -allow-command-registration")
	}
}

func TestStdio_DelegateTaskSendsExplicitZeroTemperature(t *testing.T) {
	fake := newFakeBackend(t, "delegated reply")
	c := startStdio(t, append(isolatedArgs(t.TempDir()), "-backend-url", fake.URL+"/v1", "-backend-model", "fake:1b")...)

	text, isErr := callTool(t, c, "delegate_task", map[string]any{"prompt": "hi", "temperature": 0})
	if isErr || text != "delegated reply" {
		t.Fatalf("delegate_task = %q (error=%v)", text, isErr)
	}
	if temp, ok := fake.lastRequest()["temperature"]; !ok || temp != float64(0) {
		t.Fatalf("backend saw temperature = %v (present=%v), want 0", temp, ok)
	}
}

func TestStdio_DiscoveredHostIsDefaultBackendAndUnregisterDropsIt(t *testing.T) {
	fake := newFakeBackend(t, "from discovered host")
	c := startStdio(t, isolatedArgs(t.TempDir())...)

	if text, isErr := callTool(t, c, "register_backend_host", map[string]any{"name": "fakehost", "base_url": fake.URL, "api_key": "host-secret"}); isErr {
		t.Fatalf("register_backend_host: %s", text)
	}
	text, isErr := callTool(t, c, "delegate_task", map[string]any{"prompt": "hi"})
	if isErr || text != "from discovered host" {
		t.Fatalf("delegate_task with no backend = %q (error=%v), want the discovered backend to be the default", text, isErr)
	}

	for _, tool := range []string{"list_backends", "list_backend_hosts"} {
		if out, _ := callTool(t, c, tool, nil); strings.Contains(out, "host-secret") {
			t.Fatalf("%s leaks the host API key: %s", tool, out)
		}
	}

	if text, isErr := callTool(t, c, "unregister_backend_host", map[string]any{"name": "fakehost"}); isErr {
		t.Fatalf("unregister_backend_host: %s", text)
	}
	if out, _ := callTool(t, c, "list_backends", nil); strings.Contains(out, "fakehost/") {
		t.Fatalf("list_backends after unregister still shows the host's models: %s", out)
	}
}

func TestStdio_StartsWhileScratchStoreIsHeldAndRecovers(t *testing.T) {
	// Reproduces a confirmed bug: a second process sharing the scratch
	// store (a running daemon, another MCP session) hung forever before
	// answering initialize.
	dir := t.TempDir()
	holder, err := bolt.Open(filepath.Join(dir, "scratch.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("hold scratch store: %v", err)
	}

	c := startStdio(t, isolatedArgs(dir)...) // fails the test if initialize hangs

	text, isErr := callTool(t, c, "scratch_set", map[string]any{"key": "k", "value": "v"})
	if !isErr || !strings.Contains(text, "held by another local-swarm-mcp process") {
		t.Fatalf("scratch_set while held = %q (error=%v), want a clear busy error", text, isErr)
	}

	if err := holder.Close(); err != nil {
		t.Fatalf("release scratch store: %v", err)
	}
	if text, isErr := callTool(t, c, "scratch_set", map[string]any{"key": "k", "value": "v"}); isErr {
		t.Fatalf("scratch_set after release: %s", text)
	}
	if text, _ := callTool(t, c, "scratch_get", map[string]any{"key": "k"}); text != "v" {
		t.Fatalf("scratch_get = %q, want v", text)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

type daemonProc struct {
	addr string
	dir  string
	cmd  *exec.Cmd
	out  *bytes.Buffer
}

// startDaemon runs the binary as an HTTP daemon and waits until it answers
// its health check.
func startDaemon(t *testing.T, token string, extra ...string) *daemonProc {
	t.Helper()
	d := &daemonProc{addr: freeAddr(t), dir: t.TempDir(), out: &bytes.Buffer{}}
	args := append(isolatedArgs(d.dir), "-transport", "http", "-http-addr", d.addr, "-ui", "-poll-interval", "1h")
	if token != "" {
		args = append(args, "-api-key", token)
	} else {
		args = append(args, "-insecure-no-auth")
	}
	d.cmd = exec.Command(binPath, append(args, extra...)...)
	d.cmd.Stdout, d.cmd.Stderr = d.out, d.out
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, "http://"+d.addr+"/admin/health", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return d
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("daemon never became healthy; output:\n%s", d.out.String())
	return nil
}

func (d *daemonProc) do(t *testing.T, method, path, body string, headers map[string]string) (int, string, []*http.Cookie) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://"+d.addr+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data), resp.Cookies()
}

func TestDaemon_AccessControl(t *testing.T) {
	const token = "test-token"
	d := startDaemon(t, token)
	bearer := "Bearer " + token
	jsonAuth := map[string]string{"Authorization": bearer, "Content-Type": "application/json"}

	if code, _, _ := d.do(t, http.MethodGet, "/admin/hosts", "", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /admin/hosts = %d, want 401", code)
	}
	if code, body, _ := d.do(t, http.MethodGet, "/", "", nil); code != http.StatusOK || !strings.Contains(body, "local-swarm-mcp") {
		t.Errorf("dashboard page = %d, want 200 without auth (it holds no data)", code)
	}

	textPlain := map[string]string{"Authorization": bearer, "Content-Type": "text/plain"}
	if code, _, _ := d.do(t, http.MethodPost, "/admin/register-host", `{"name":"x","base_url":"http://127.0.0.1:1"}`, textPlain); code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain POST = %d, want 415", code)
	}

	crossSite := map[string]string{"Authorization": bearer, "Content-Type": "application/json", "Origin": "https://evil.example"}
	if code, _, _ := d.do(t, http.MethodPost, "/admin/register-host", `{"name":"x","base_url":"http://127.0.0.1:1"}`, crossSite); code != http.StatusForbidden {
		t.Errorf("cross-origin POST = %d, want 403", code)
	}

	rebound := map[string]string{"Authorization": bearer, "Host": "evil.example"}
	if code, _, _ := d.do(t, http.MethodGet, "/admin/hosts", "", rebound); code != http.StatusForbidden {
		t.Errorf("DNS-rebound Host on loopback = %d, want 403", code)
	}

	if code, _, _ := d.do(t, http.MethodPost, "/admin/register-mcp-server", `{"name":"x","command":"calc.exe"}`, jsonAuth); code != http.StatusForbidden {
		t.Errorf("register-mcp-server without -allow-command-registration = %d, want 403", code)
	}

	code, body, _ := d.do(t, http.MethodPost, "/admin/register-host", `{"name":"h","base_url":"http://127.0.0.1:1","api_key":"host-secret"}`, jsonAuth)
	if code != http.StatusOK || strings.Contains(body, "host-secret") {
		t.Errorf("register-host = %d %s, want 200 without echoing the key", code, body)
	}
	if _, body, _ := d.do(t, http.MethodGet, "/admin/hosts", "", jsonAuth); strings.Contains(body, "host-secret") || !strings.Contains(body, `"has_api_key":true`) {
		t.Errorf("/admin/hosts = %s, want the key redacted", body)
	}
}

func TestDaemon_DashboardLoginCookie(t *testing.T) {
	const token = "test-token"
	d := startDaemon(t, token)
	jsonOnly := map[string]string{"Content-Type": "application/json"}

	if code, _, _ := d.do(t, http.MethodPost, "/admin/login", `{"token":"wrong"}`, jsonOnly); code != http.StatusUnauthorized {
		t.Fatalf("login with a wrong token = %d, want 401", code)
	}
	code, _, cookies := d.do(t, http.MethodPost, "/admin/login", `{"token":"`+token+`"}`, jsonOnly)
	if code != http.StatusNoContent || len(cookies) != 1 {
		t.Fatalf("login = %d, cookies %v", code, cookies)
	}
	cookie := cookies[0]

	req, _ := http.NewRequest(http.MethodGet, "http://"+d.addr+"/admin/hosts", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with cookie: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/admin/hosts with session cookie = %d, want 200", resp.StatusCode)
	}

	// The cookie authenticates the dashboard only, not the MCP endpoint
	// as a bearer replacement from outside a browser.
	mcpReq, _ := http.NewRequest(http.MethodPost, "http://"+d.addr+"/mcp", strings.NewReader(`{}`))
	mcpReq.Header.Set("Authorization", "Bearer "+cookie.Value)
	resp, err = http.DefaultClient.Do(mcpReq)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/mcp with the cookie value as a bearer token = %d, want 401", resp.StatusCode)
	}
}

func runBinary(t *testing.T, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binPath, args...).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%v timed out after %s (hung); output:\n%s", args, timeout, out)
	}
	return string(out), err
}

func TestRegisterHost_ForwardsToRunningDaemonWithoutHanging(t *testing.T) {
	// Reproduces a confirmed bug: -register-host against a running daemon
	// hung opening the daemon's scratch store before ever forwarding.
	const token = "test-token"
	d := startDaemon(t, token)
	base := append(isolatedArgs(d.dir), "-http-addr", d.addr, "-register-host", "-name", "late", "-host-base-url", "http://127.0.0.1:1")

	if out, err := runBinary(t, 15*time.Second, append(base, "-api-key", token)...); err != nil {
		t.Fatalf("-register-host: %v\n%s", err, out)
	}
	if _, body, _ := d.do(t, http.MethodGet, "/admin/hosts", "", map[string]string{"Authorization": "Bearer " + token}); !strings.Contains(body, `"name":"late"`) {
		t.Fatalf("forwarded host missing from the daemon: %s", body)
	}

	out, err := runBinary(t, 15*time.Second, base...)
	if err == nil || !strings.Contains(out, "-api-key") {
		t.Fatalf("-register-host without the daemon's token = %v\n%s; want an error pointing at -api-key", err, out)
	}
}

func TestHTTP_RefusesUnauthenticatedNetworkExposure(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]string{
		"no auth at all":                {"-transport", "http", "-http-addr", freeAddr(t)},
		"insecure on every interface":   {"-transport", "http", "-http-addr", ":0", "-insecure-no-auth"},
		"insecure on an explicit 0.0.0": {"-transport", "http", "-http-addr", "0.0.0.0:0", "-insecure-no-auth"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runBinary(t, 15*time.Second, append(isolatedArgs(dir), args...)...)
			if err == nil {
				t.Fatalf("expected the daemon to refuse to start; output:\n%s", out)
			}
		})
	}
}
