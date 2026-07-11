package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jallum/beadwork/internal/signal"
	"github.com/jallum/beadwork/internal/testutil"
	"github.com/jallum/beadwork/internal/treefs"
)

func TestSignalEmitUndefinedTypeRefused(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
`)
	var buf bytes.Buffer
	_, err := cmdSignal(env.Store, []string{"emit", "test-x", "deferral"}, PlainWriter(&buf), nil)
	if err == nil {
		t.Fatal("expected undefined type refusal")
	}
	if !strings.Contains(err.Error(), "VALIDATION") || !strings.Contains(err.Error(), "deferral") {
		t.Fatalf("err = %q, want VALIDATION naming undefined type", err.Error())
	}
	entries, readErr := env.Store.FS.ReadDir("signals/test-x")
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("signal should not have been stored: %#v", entries)
	}
}

func TestSignalHistoryOutlivesDefinitions(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
`)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"emit", "test-x", "verify", "--field", "phase=PASS"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("cmdSignal emit: %v", err)
	}
	writeCmdSignals(t, env.Dir, `types: {}`)

	records, err := env.Store.SignalsForTicket("test-x")
	if err != nil {
		t.Fatalf("SignalsForTicket: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Type != "verify" || records[0].Payload["phase"] != "PASS" {
		t.Fatalf("record changed after type removal: %#v", records[0])
	}
}

func TestUsageListsSignalGroup(t *testing.T) {
	var buf bytes.Buffer
	printUsage(PlainWriter(&buf))
	out := buf.String()
	if !strings.Contains(out, "Signals:") || !strings.Contains(out, "signal emit|types") {
		t.Fatalf("usage missing signal group/command:\n%s", out)
	}
}

func TestSignalRequiredWhenRouting(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeCmdSignals(t, env.Dir, `
types:
  audit:
    fields:
      phase:
        type: enum
        values: [APPROVE, BOUNCE]
        required: true
      target:
        type: enum
        values: [implementer, verifier]
        required_when:
          field: phase
          equals: BOUNCE
`)
	var buf bytes.Buffer
	_, err := cmdSignal(env.Store, []string{"emit", "test-x", "audit", "--field", "phase=BOUNCE"}, PlainWriter(&buf), nil)
	if err == nil {
		t.Fatal("expected required_when validation error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Fatalf("err = %q, want target", err.Error())
	}
}

func writeCmdSignals(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".beadwork")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "signals.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestSignalEmitNoBypassFlag(t *testing.T) {
	for _, flag := range []string{"--no-hooks", "--skip-hooks", "--force"} {
		_, err := parseSignalArgs([]string{"emit", "test-x", "verify", flag})
		if err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("flag %s: %v", flag, err)
		}
	}
}

func TestSignalEmitHelpMentionsPipeline(t *testing.T) {
	for _, command := range commands {
		if command.Name == "signal" {
			if !strings.Contains(command.Description, "pipeline") || !strings.Contains(command.Description, "no bypass") {
				t.Fatalf("description=%q", command.Description)
			}
			return
		}
	}
	t.Fatal("signal command missing")
}

func TestSignalHooksRunOnceUnderRetry(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	count := filepath.Join(env.Dir, "count")
	hook := filepath.Join(env.Dir, "hook")
	body := "#!/bin/sh\necho x >> '" + count + "'\ncat >/dev/null\n"
	if err := os.WriteFile(hook, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	typ := &signal.Type{Name: "verify", Hooks: signal.Hooks{Gate: []string{hook}}}
	cfg := &signal.Config{HookTimeout: time.Second}
	racer, err := treefs.Open(env.Dir, "refs/heads/beadwork")
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	_, _, err = (signal.Pipeline{RepoRoot: env.Dir, Config: cfg, Type: typ, Ticket: "test-x"}).Run(map[string]any{}, func(p map[string]any) (map[string]any, error) { return p, nil }, func(p map[string]any) error {
		return commitWithRetry(env.Store, 3, func() (string, error) {
			attempts++
			if _, _, err := env.Store.EmitSignal("test-x", "verify", p); err != nil {
				return "", err
			}
			if attempts == 1 {
				if err := racer.WriteFile("racer", []byte("x")); err != nil {
					return "", err
				}
				if err := racer.Commit("racer"); err != nil {
					return "", err
				}
			}
			return "signal test-x verify signals/test-x/0001.json", nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts < 2 {
		t.Fatalf("attempts=%d", attempts)
	}
	b, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(b))); got != 1 {
		t.Fatalf("hook calls=%d", got)
	}
}
