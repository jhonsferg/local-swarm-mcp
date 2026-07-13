package hostregistry

import (
	"errors"
	"path/filepath"
	"testing"
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

func TestUnregisterHost_RemovesHostAndModels(t *testing.T) {
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

func TestRecordPoll_SuccessUpdatesStatusAndPersistsModels(t *testing.T) {
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
		t.Fatalf("st.Models = %+v, want the last known models preserved", st.Models)
	}
}

func TestOpen_ReloadsPreviouslyRegisteredHosts(t *testing.T) {
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
		t.Fatalf("StatusOf after reopen: not found")
	}
	if len(st.Models) != 1 || st.Models[0].Name != "gemma4:12b" {
		t.Fatalf("st.Models after reopen = %+v, want [gemma4:12b]", st.Models)
	}
}
