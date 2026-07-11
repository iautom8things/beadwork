package signal_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jallum/beadwork/internal/signal"
)

func TestConfigWorkingTreeSource(t *testing.T) {
	dir := t.TempDir()
	writeSignals(t, dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
`)
	cfg, err := signal.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.TypeByName("verify"); !ok {
		t.Fatalf("working-tree config did not define verify: %#v", cfg)
	}
}

func TestConfigFailClosedMalformed(t *testing.T) {
	dir := t.TempDir()
	writeSignals(t, dir, `
types:
  - name: verify
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
  - name: verify
`)
	_, err := signal.Load(dir)
	if err == nil {
		t.Fatal("expected duplicate type to fail")
	}
	var cfgErr signal.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %T %[1]v, want ConfigError", err)
	}
	if !strings.Contains(err.Error(), "CONFIG ERROR") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %q, want CONFIG ERROR duplicate", err.Error())
	}
}

func TestSchemaExpressiveness(t *testing.T) {
	cfg, err := signal.Parse([]byte(`
types:
  implemented: {}
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
      retries:
        type: int
      fresh:
        type: bool
      note:
        type: string
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if typ, ok := cfg.TypeByName("implemented"); !ok || len(typ.Fields) != 0 {
		t.Fatalf("implemented pure token = %#v, %v", typ, ok)
	}
	typ, ok := cfg.TypeByName("audit")
	if !ok {
		t.Fatal("missing audit type")
	}
	if len(typ.Fields) != 5 {
		t.Fatalf("fields = %d, want 5", len(typ.Fields))
	}
}

func TestValidateFinalPayload(t *testing.T) {
	typ := signal.Type{
		Name: "audit",
		Fields: []signal.Field{
			{Name: "phase", Kind: "enum", Values: []string{"APPROVE", "BOUNCE"}, Required: true},
			{Name: "target", Kind: "enum", Values: []string{"implementer", "verifier"}, RequiredWhen: &signal.RequiredWhen{Field: "phase", Equals: "BOUNCE"}},
			{Name: "attempt", Kind: "int"},
			{Name: "fresh", Kind: "bool"},
		},
	}
	got, err := signal.ValidatePayload(typ, map[string]any{
		"phase":   "BOUNCE",
		"target":  "verifier",
		"attempt": "2",
		"fresh":   "true",
	})
	if err != nil {
		t.Fatalf("ValidatePayload: %v", err)
	}
	if got["attempt"] != 2 || got["fresh"] != true {
		t.Fatalf("coerced payload = %#v", got)
	}

	_, err = signal.ValidatePayload(typ, map[string]any{"phase": "BOUNCE"})
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("required_when err = %v, want missing target", err)
	}
	_, err = signal.ValidatePayload(typ, map[string]any{"phase": "MAYBE"})
	if err == nil || !strings.Contains(err.Error(), "APPROVE") {
		t.Fatalf("enum err = %v, want allowed values", err)
	}
}

func writeSignals(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".beadwork")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "signals.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
