package main

import (
	"bytes"
	"encoding/json"
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

func TestSignalTypesVerbose(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	enrichGlobal := filepath.Join(env.Dir, "global-enrich")
	enrichType := filepath.Join(env.Dir, "type-enrich")
	gateType := filepath.Join(env.Dir, "type-gate")
	gateGlobal := filepath.Join(env.Dir, "global-gate")
	writeCmdSignals(t, env.Dir, `
hook_timeout: 2s
hooks:
  enrich: `+enrichGlobal+`
  gate: `+gateGlobal+`
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
    hooks:
      enrich: `+enrichType+`
      gate: `+gateType+`
`)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"types", "--verbose"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("cmdSignal types --verbose: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"audit",
		"phase: enum [APPROVE, BOUNCE] required",
		"target: enum [implementer, verifier] required_when phase=BOUNCE",
		"hook_timeout: 2s",
		"global: " + enrichGlobal,
		"type: " + enrichType,
		"type: " + gateType,
		"global: " + gateGlobal,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("verbose output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "global: "+enrichGlobal) > strings.Index(out, "type: "+enrichType) {
		t.Fatalf("enrich hooks not rendered global before type:\n%s", out)
	}
	if strings.Index(out, "type: "+gateType) > strings.Index(out, "global: "+gateGlobal) {
		t.Fatalf("gate hooks not rendered type before global:\n%s", out)
	}
}

func TestSignalTypesPlainOutputContract(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields: {}
  audit:
    fields: {}
  completed:
    fields: {}
`)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"types"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("cmdSignal types: %v", err)
	}
	if got, want := buf.String(), "audit\ncompleted\nverify\n"; got != want {
		t.Fatalf("plain types output = %q, want %q", got, want)
	}
}

func TestSignalTypesJSON(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeIntrospectionSignals(t, env.Dir)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"types", "--json"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("cmdSignal types --json: %v", err)
	}
	var got struct {
		Types []signalTypeDetail `json:"types"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json parse: %v\n%s", err, buf.String())
	}
	if len(got.Types) != 1 || got.Types[0].Name != "audit" {
		t.Fatalf("types = %#v", got.Types)
	}
	if got.Types[0].Fields[1].RequiredWhen == nil || got.Types[0].Fields[1].RequiredWhen.Field != "phase" {
		t.Fatalf("required_when missing from json: %#v", got.Types[0].Fields)
	}
	if got.Types[0].Hooks.Enrich[0].Source != "global" || got.Types[0].Hooks.Enrich[1].Source != "type" {
		t.Fatalf("enrich order = %#v", got.Types[0].Hooks.Enrich)
	}
	if got.Types[0].Hooks.Gate[0].Source != "type" || got.Types[0].Hooks.Gate[1].Source != "global" {
		t.Fatalf("gate order = %#v", got.Types[0].Hooks.Gate)
	}
}

func TestSignalShowType(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeIntrospectionSignals(t, env.Dir)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"show", "audit"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("cmdSignal show: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "audit") || !strings.Contains(out, "phase: enum [APPROVE, BOUNCE] required") {
		t.Fatalf("show output missing detail:\n%s", out)
	}
}

func TestSignalShowUndefinedType(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeIntrospectionSignals(t, env.Dir)
	var buf bytes.Buffer
	_, err := cmdSignal(env.Store, []string{"show", "missing"}, PlainWriter(&buf), nil)
	if err == nil || !strings.Contains(err.Error(), "VALIDATION") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v, want validation error naming undefined type", err)
	}
}

func TestSignalValidateSchemaOnly(t *testing.T) {
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
	var valid bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"validate", "verify", "--field", "phase=PASS"}, PlainWriter(&valid), nil); err != nil {
		t.Fatalf("validate valid: %v\n%s", err, valid.String())
	}
	if !strings.Contains(valid.String(), "schema: PASS") || !strings.Contains(valid.String(), "field phase: PASS") {
		t.Fatalf("valid report missing pass:\n%s", valid.String())
	}
	var invalid bytes.Buffer
	_, err := cmdSignal(env.Store, []string{"validate", "verify", "--field", "phase=MAYBE"}, PlainWriter(&invalid), nil)
	if err == nil {
		t.Fatal("expected invalid payload to fail")
	}
	if !strings.Contains(invalid.String(), "schema: FAIL") || !strings.Contains(err.Error(), "MAYBE") {
		t.Fatalf("invalid report/error missing failure:\nreport=%s\nerr=%v", invalid.String(), err)
	}
}

func TestSignalValidateNeverStores(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	hook := writeHook(t, env.Dir, "enrich", `#!/bin/sh
cat >/dev/null
printf '{"phase":"PASS","stamp":"enriched"}'
`)
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
      stamp:
        type: string
    hooks:
      enrich: `+hook+`
`)
	for _, args := range [][]string{
		{"validate", "verify", "--field", "phase=PASS"},
		{"validate", "verify", "--run-hooks"},
	} {
		var buf bytes.Buffer
		if _, err := cmdSignal(env.Store, args, PlainWriter(&buf), nil); err != nil {
			t.Fatalf("cmdSignal %v: %v\n%s", args, err, buf.String())
		}
		if hasRunHooks(args) && !strings.Contains(buf.String(), `"stamp":"enriched"`) {
			t.Fatalf("dry-run report missing enriched payload stamp:\n%s", buf.String())
		}
		records, err := env.Store.SignalsForTicket("test-x")
		if err != nil {
			t.Fatalf("SignalsForTicket: %v", err)
		}
		if len(records) != 0 {
			t.Fatalf("validate %v stored records: %#v", args, records)
		}
	}
}

func TestSignalValidateRunHooksSuppressesPostEmit(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	postEmitMarker := filepath.Join(env.Dir, "post-emit-marker")
	postEmit := writeHook(t, env.Dir, "post-emit", `#!/bin/sh
cat >/dev/null
echo fired > '`+postEmitMarker+`'
`)
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
    hooks:
      post-emit: `+postEmit+`
`)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"validate", "verify", "--field", "phase=PASS", "--run-hooks"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("validate --run-hooks: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "gate: ALLOWED") {
		t.Fatalf("allowed dry-run report missing gate verdict:\n%s", buf.String())
	}
	if _, err := os.Stat(postEmitMarker); !os.IsNotExist(err) {
		t.Fatalf("post-emit fired during dry-run, stat err=%v", err)
	}
	records, err := env.Store.SignalsForTicket("test-x")
	if err != nil {
		t.Fatalf("SignalsForTicket: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("dry-run allowed validate stored records: %#v", records)
	}
}

func TestSignalValidateRunHooksGateBlock(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	blockedMarker := filepath.Join(env.Dir, "blocked-marker")
	onBlockedMarker := filepath.Join(env.Dir, "on-blocked-marker")
	gate := writeHook(t, env.Dir, "gate", `#!/bin/sh
cat >/dev/null
echo policy-blocked > '`+blockedMarker+`'
echo policy-blocked
exit 1
`)
	onBlocked := writeHook(t, env.Dir, "on-blocked", `#!/bin/sh
cat >/dev/null
echo fired > '`+onBlockedMarker+`'
`)
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
    hooks:
      gate: `+gate+`
      on-blocked: `+onBlocked+`
`)
	var buf bytes.Buffer
	_, err := cmdSignal(env.Store, []string{"validate", "verify", "--field", "phase=PASS", "--run-hooks"}, PlainWriter(&buf), nil)
	if err == nil {
		t.Fatal("expected gate block")
	}
	if !strings.Contains(buf.String(), "gate: BLOCKED") || !strings.Contains(buf.String(), "policy-blocked") {
		t.Fatalf("blocked report missing verdict/reason:\n%s", buf.String())
	}
	if _, err := os.Stat(blockedMarker); err != nil {
		t.Fatalf("gate marker missing: %v", err)
	}
	if _, err := os.Stat(onBlockedMarker); !os.IsNotExist(err) {
		t.Fatalf("on-blocked fired during dry-run, stat err=%v", err)
	}
	records, err := env.Store.SignalsForTicket("test-x")
	if err != nil {
		t.Fatalf("SignalsForTicket: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("dry-run gate block stored records: %#v", records)
	}
}

func TestSignalValidateDryRunEnv(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	seen := filepath.Join(env.Dir, "dry-run-env")
	hook := writeHook(t, env.Dir, "gate-env", `#!/bin/sh
cat >/dev/null
printf '%s' "$BW_SIGNAL_DRY_RUN" > '`+seen+`'
`)
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      phase:
        type: enum
        values: [PASS, FAIL]
        required: true
    hooks:
      gate: `+hook+`
`)
	var buf bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"validate", "verify", "--field", "phase=PASS", "--run-hooks"}, PlainWriter(&buf), nil); err != nil {
		t.Fatalf("validate --run-hooks: %v\n%s", err, buf.String())
	}
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("read env marker: %v", err)
	}
	if string(data) != "1" {
		t.Fatalf("BW_SIGNAL_DRY_RUN = %q, want 1", string(data))
	}
}

func writeIntrospectionSignals(t *testing.T, dir string) {
	t.Helper()
	writeCmdSignals(t, dir, `
hook_timeout: 2s
hooks:
  enrich: /global-enrich
  gate: /global-gate
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
    hooks:
      enrich: /type-enrich
      gate: /type-gate
`)
}

func writeHook(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("WriteFile hook: %v", err)
	}
	return path
}

func hasRunHooks(args []string) bool {
	for _, arg := range args {
		if arg == "--run-hooks" {
			return true
		}
	}
	return false
}
