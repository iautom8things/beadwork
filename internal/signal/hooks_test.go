package signal_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jallum/beadwork/internal/signal"
)

func hook(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hook")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func pipeline(root string, cfg *signal.Config, typ *signal.Type) signal.Pipeline {
	return signal.Pipeline{RepoRoot: root, Config: cfg, Type: typ, Ticket: "bw-x"}
}

func TestPipelineEmitSequence(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	mk := func(moment string) string { return hook(t, "echo "+moment+" >> '"+log+"'; cat >/dev/null") }
	typ := &signal.Type{Name: "done", Hooks: signal.Hooks{Enrich: []string{mk("type-enrich")}, Gate: []string{mk("type-gate")}, PostEmit: []string{mk("type-post")}}}
	cfg := &signal.Config{HookTimeout: time.Second, Hooks: signal.Hooks{Enrich: []string{mk("global-enrich")}, Gate: []string{mk("global-gate")}, PostEmit: []string{mk("global-post")}}}
	_, _, err := pipeline(dir, cfg, typ).Run(map[string]any{}, func(p map[string]any) (map[string]any, error) {
		os.WriteFile(log, append(read(t, log), []byte("validate\n")...), 0644)
		return p, nil
	}, func(map[string]any) error {
		os.WriteFile(log, append(read(t, log), []byte("store\n")...), 0644)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(read(t, log)))
	want := []string{"global-enrich", "type-enrich", "validate", "type-gate", "global-gate", "store", "type-post", "global-post"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sequence=%v want %v", got, want)
	}
}

func TestHookStdinJSONContract(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "stdin")
	h := hook(t, "cat > '"+captured+"'; printf '{\"stamped\":true}'")
	typ := &signal.Type{Name: "verify", Hooks: signal.Hooks{Enrich: []string{h}}}
	cfg := &signal.Config{HookTimeout: time.Second}
	got, _, err := pipeline(dir, cfg, typ).Run(map[string]any{"nested": map[string]any{"x": "y"}}, func(p map[string]any) (map[string]any, error) { return p, nil }, func(map[string]any) error { return nil })
	if err != nil || got["stamped"] != true {
		t.Fatalf("got=%v err=%v", got, err)
	}
	var rec struct {
		Type, Ticket string
		Payload      map[string]any
	}
	if err := json.Unmarshal(read(t, captured), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Type != "verify" || rec.Ticket != "bw-x" || rec.Payload["nested"] == nil {
		t.Fatalf("record=%#v", rec)
	}
}

func TestOnBlockedFailureIsolated(t *testing.T) {
	block := hook(t, "echo no; exit 1")
	crash := hook(t, "exit 2")
	typ := &signal.Type{Name: "x", Hooks: signal.Hooks{Gate: []string{block}, OnBlocked: []string{crash}}}
	cfg := &signal.Config{HookTimeout: time.Second}
	_, warnings, err := pipeline(t.TempDir(), cfg, typ).Run(map[string]any{}, func(p map[string]any) (map[string]any, error) { return p, nil }, func(map[string]any) error { t.Fatal("stored"); return nil })
	var he signal.HookError
	if !errors.As(err, &he) || he.Kind != "BLOCKED" || len(warnings) != 1 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
}

func TestHookTimeoutKilled(t *testing.T) {
	h := hook(t, "sleep 999")
	typ := &signal.Type{Name: "x", Hooks: signal.Hooks{Gate: []string{h}}}
	cfg := &signal.Config{HookTimeout: 25 * time.Millisecond}
	start := time.Now()
	_, _, err := pipeline(t.TempDir(), cfg, typ).Run(map[string]any{}, func(p map[string]any) (map[string]any, error) { return p, nil }, func(map[string]any) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "MALFUNCTION") || time.Since(start) > time.Second {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return b
}
