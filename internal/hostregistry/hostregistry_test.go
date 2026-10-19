package hostregistry

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func openTestRegistry(t *testing.T) *Registry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts.db")
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRegisterAndListHosts(t *testing.T) {
	r := openTestRegistry(t)

	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434", APIKey: "secret"}
	if err := r.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	hosts, err := r.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != host {
		t.Fatalf("Hosts() = %+v, want [%+v]", hosts, host)
	}
}

func TestUnregisterHost_RemovesHost(t *testing.T) {
	r := openTestRegistry(t)
	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}
	if err := r.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	if err := r.RecordPoll(host.Name, []Model{{Name: "gemma4:12b"}}, nil); err != nil {
		t.Fatalf("RecordPoll: %v", err)
	}

	if err := r.UnregisterHost(host.Name); err != nil {
		t.Fatalf("UnregisterHost: %v", err)
	}

	hosts, err := r.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("Hosts() after unregister = %+v, want empty", hosts)
	}
	if _, ok := r.StatusOf(host.Name); ok {
		t.Fatalf("StatusOf still reports the unregistered host")
	}
}

func TestRecordPoll_SuccessUpdatesStatus(t *testing.T) {
	r := openTestRegistry(t)
	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}
	if err := r.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	models := []Model{{Name: "gemma4:12b", Capabilities: []string{"tools"}}}
	if err := r.RecordPoll(host.Name, models, nil); err != nil {
		t.Fatalf("RecordPoll: %v", err)
	}

	st, ok := r.StatusOf(host.Name)
	if !ok {
		t.Fatalf("StatusOf: not found")
	}
	if !st.Up {
		t.Fatalf("st.Up = false, want true after a successful poll")
	}
	if len(st.Models) != 1 || st.Models[0].Name != "gemma4:12b" {
		t.Fatalf("st.Models = %+v, want [gemma4:12b]", st.Models)
	}
	if st.LastErr != "" {
		t.Fatalf("st.LastErr = %q, want empty", st.LastErr)
	}
}

func TestRecordPoll_SuccessReplacesModelsEntirely(t *testing.T) {
	// A model no longer present on the host (e.g. deleted locally) must
	// stop showing up as of the very next successful poll - nothing here
	// should linger from an earlier poll's result.
	r := openTestRegistry(t)
	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}
	if err := r.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	if err := r.RecordPoll(host.Name, []Model{{Name: "gemma4:12b"}, {Name: "qwen3.5:latest"}}, nil); err != nil {
		t.Fatalf("RecordPoll (first): %v", err)
	}
	if err := r.RecordPoll(host.Name, []Model{{Name: "gemma4:12b"}}, nil); err != nil {
		t.Fatalf("RecordPoll (second): %v", err)
	}

	st, ok := r.StatusOf(host.Name)
	if !ok {
		t.Fatalf("StatusOf: not found")
	}
	if len(st.Models) != 1 || st.Models[0].Name != "gemma4:12b" {
		t.Fatalf("st.Models = %+v, want only [gemma4:12b] - qwen3.5:latest should be gone", st.Models)
	}
}

func TestRecordPoll_FailureMarksDownButKeepsLastKnownModels(t *testing.T) {
	r := openTestRegistry(t)
	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}
	if err := r.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	if err := r.RecordPoll(host.Name, []Model{{Name: "gemma4:12b"}}, nil); err != nil {
		t.Fatalf("RecordPoll (success): %v", err)
	}

	pollErr := errors.New("connection refused")
	if err := r.RecordPoll(host.Name, nil, pollErr); err != nil {
		t.Fatalf("RecordPoll (failure): %v", err)
	}

	st, ok := r.StatusOf(host.Name)
	if !ok {
		t.Fatalf("StatusOf: not found")
	}
	if st.Up {
		t.Fatalf("st.Up = true, want false after a failed poll")
	}
	if st.LastErr != pollErr.Error() {
		t.Fatalf("st.LastErr = %q, want %q", st.LastErr, pollErr.Error())
	}
	if len(st.Models) != 1 || st.Models[0].Name != "gemma4:12b" {
		t.Fatalf("st.Models = %+v, want the last known models preserved in memory", st.Models)
	}
}

func TestOpen_ReloadsHostsButNotModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.db")

	r1, err := Open(path)
	if err != nil {
		t.Fatalf("Open (1st): %v", err)
	}
	host := Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}
	if err := r1.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	if err := r1.RecordPoll(host.Name, []Model{{Name: "gemma4:12b"}}, nil); err != nil {
		t.Fatalf("RecordPoll: %v", err)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r2, err := Open(path)
	if err != nil {
		t.Fatalf("Open (2nd): %v", err)
	}
	defer func() { _ = r2.Close() }()

	st, ok := r2.StatusOf(host.Name)
	if !ok {
		t.Fatalf("StatusOf after reopen: host not found")
	}
	if st.BaseURL != host.BaseURL {
		t.Fatalf("st.BaseURL after reopen = %q, want %q", st.BaseURL, host.BaseURL)
	}
	// Models are never persisted - only a real poll of the host's own API
	// should ever populate them, never a restart reloading old state.
	if len(st.Models) != 0 {
		t.Fatalf("st.Models after reopen = %+v, want empty (models are never persisted)", st.Models)
	}
}

// TestOpenWithOptions_TimesOutWhenAlreadyLocked confirms a caller can bound
// how long it waits for another process's file lock - exercising the stdio
// session's opportunistic (non-blocking-forever) attempt to share the
// daemon's store, instead of hanging indefinitely as a nil-options Open
// would.
func TestOpenWithOptions_TimesOutWhenAlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.db")

	holder, err := Open(path)
	if err != nil {
		t.Fatalf("Open (holder): %v", err)
	}
	defer func() { _ = holder.Close() }()

	start := time.Now()
	_, err = OpenWithOptions(path, &bolt.Options{Timeout: 100 * time.Millisecond})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("OpenWithOptions succeeded despite the file already being held open")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("OpenWithOptions took %s to fail - timeout was not respected", elapsed)
	}
}

func TestRecordPoll_UnknownHostIsDiscarded(t *testing.T) {
	r := openTestRegistry(t)

	err := r.RecordPoll("gone", []Model{{Name: "gemma4:12b"}}, nil)
	if !errors.Is(err, ErrUnknownHost) {
		t.Fatalf("RecordPoll(unregistered) = %v, want ErrUnknownHost", err)
	}
	if _, ok := r.StatusOf("gone"); ok {
		t.Fatal("RecordPoll must not create a status entry for an unregistered host")
	}
}

func TestUpsert_EmptyKeyKeepsExistingKey(t *testing.T) {
	r := openTestRegistry(t)
	if err := r.RegisterHost(Host{Name: "rx9070", BaseURL: "http://a:11434", APIKey: "k1"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	stored, err := r.Upsert(Host{Name: "rx9070", BaseURL: "http://b:11434"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if stored.APIKey != "k1" || stored.BaseURL != "http://b:11434" {
		t.Fatalf("Upsert returned %+v, want new base URL with the existing key", stored)
	}
	hosts, err := r.Hosts()
	if err != nil || len(hosts) != 1 || hosts[0] != stored {
		t.Fatalf("Hosts() = %+v, %v; want [%+v]", hosts, err, stored)
	}

	if stored, err = r.Upsert(Host{Name: "rx9070", BaseURL: "http://b:11434", APIKey: "k2"}); err != nil || stored.APIKey != "k2" {
		t.Fatalf("Upsert with a new key = %+v, %v; want key k2", stored, err)
	}
}

func TestView_RedactsAPIKey(t *testing.T) {
	r := openTestRegistry(t)
	if err := r.RegisterHost(Host{Name: "rx9070", BaseURL: "http://a:11434", APIKey: "super-secret"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	views := r.Views()
	if len(views) != 1 || !views[0].HasAPIKey {
		t.Fatalf("Views() = %+v, want one host with HasAPIKey", views)
	}
	data, _ := json.Marshal(views)
	if strings.Contains(string(data), "super-secret") || strings.Contains(string(data), `"api_key"`) {
		t.Fatalf("Views() JSON leaks the key: %s", data)
	}
}
