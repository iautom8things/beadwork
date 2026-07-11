package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jallum/beadwork/internal/testutil"
)

func TestSignalQueryStoreAssignedCursor(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	writeCmdSignals(t, env.Dir, `
types:
  verify:
    fields:
      timestamp:
        type: string
  audit: {}
`)
	for _, timestamp := range []string{"9999-12-31", "0001-01-01"} {
		var out bytes.Buffer
		_, err := cmdSignal(env.Store, []string{"emit", "test-x", "verify", "--field", "timestamp=" + timestamp}, PlainWriter(&out), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	var ignored bytes.Buffer
	if _, err := cmdSignal(env.Store, []string{"emit", "test-x", "audit"}, PlainWriter(&ignored), nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := cmdSignalWithQuery(env.Store, []string{"query", "--ticket", "test-x", "--type", "verify", "--json"}, PlainWriter(&out), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got SignalQueryResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Signals) != 2 || got.Signals[0].Seq != 1 || got.Signals[1].Seq != 2 {
		t.Fatalf("signals=%#v, want only filtered verify sequence 1,2", got.Signals)
	}
	if got.Cursor != env.Repo.TreeFS().RefHash().String() {
		t.Fatalf("cursor=%s, want current head", got.Cursor)
	}
}

func TestPrimeMentionsSignalsWhenDefined(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()
	var dormant bytes.Buffer
	if _, err := cmdPrime(env.Store, nil, PlainWriter(&dormant), nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dormant.String(), "Signals are typed") {
		t.Fatal("dormant prime unexpectedly mentions signals")
	}
	dir := filepath.Join(env.Dir, ".beadwork")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signals.yml"), []byte("types:\n  verify: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var active bytes.Buffer
	if _, err := cmdPrime(env.Store, nil, PlainWriter(&active), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(active.String(), "Signals are typed") {
		t.Fatal("active prime missing signals paragraph")
	}
}
