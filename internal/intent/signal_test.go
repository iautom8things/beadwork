package intent_test

import (
	"strings"
	"testing"

	"github.com/jallum/beadwork/internal/intent"
	"github.com/jallum/beadwork/internal/testutil"
)

func TestReplaySignal(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()

	_, path, err := env.Store.EmitSignal("test-x", "verify", map[string]any{"phase": "PASS"})
	if err != nil {
		t.Fatalf("EmitSignal: %v", err)
	}
	env.CommitIntent("signal test-x verify " + path)
	preReset := env.Repo.TreeFS().RefHash()

	commits, err := env.Repo.AllCommits()
	if err != nil || len(commits) < 2 {
		t.Fatalf("AllCommits: %v (got %d)", err, len(commits))
	}
	resetTo := commitHashFromString(t, env, commits[len(commits)-1].Hash)
	if err := env.Repo.TreeFS().Reset(resetTo); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	env.Store.ClearCache()
	if _, err := env.Repo.TreeFS().ReadFile(path); err == nil {
		t.Fatal("signal should not be reachable after reset")
	}

	env.Store.SourceHash = preReset
	errs := intent.Replay(env.Store, []string{"signal test-x verify " + path})
	if len(errs) != 0 {
		t.Fatalf("Replay errors: %v", errs)
	}
	got, err := env.Repo.TreeFS().ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after replay: %v", err)
	}
	if !strings.Contains(string(got), `"phase": "PASS"`) {
		t.Fatalf("replayed blob = %s", got)
	}

	errs = intent.Replay(env.Store, []string{"signal test-missing verify signals/test-missing/0001.json"})
	if len(errs) != 1 {
		t.Fatalf("missing blob errors = %d, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "signal") {
		t.Fatalf("missing blob error = %v, want signal context", errs[0])
	}
}
