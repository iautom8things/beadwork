package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jallum/beadwork/internal/testutil"
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
