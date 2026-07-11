package issue_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
	"github.com/jallum/beadwork/internal/testutil"
	"github.com/jallum/beadwork/internal/treefs"
)

func TestSignalConcurrentEmits(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- emitSignalWithRetry(env.Dir, "test-x")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("emit/commit: %v", err)
		}
	}

	records, err := env.Store.SignalsForTicket("test-x")
	if err != nil {
		t.Fatalf("SignalsForTicket: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2: %#v", len(records), records)
	}
	seen := map[int]bool{}
	for _, rec := range records {
		seen[rec.Seq] = true
		if rec.Type != "verify" || rec.Ticket != "test-x" || rec.EmittedAt == "" {
			t.Fatalf("bad record: %#v", rec)
		}
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("sequences = %#v, want 1 and 2", seen)
	}
}

func emitSignalWithRetry(repoDir, ticketID string) error {
	r, err := repo.FindRepoAt(repoDir)
	if err != nil {
		return err
	}
	store := issue.NewStore(r.TreeFS(), r.Prefix)
	store.Committer = r
	for attempt := 0; attempt < 12; attempt++ {
		if attempt > 0 {
			if err := store.ReopenFS(); err != nil {
				return err
			}
		}
		_, path, err := store.EmitSignal(ticketID, "verify", map[string]any{"phase": "PASS"})
		if err != nil {
			return err
		}
		err = store.Commit("signal " + ticketID + " verify " + path)
		if err == nil {
			return nil
		}
		if !errors.Is(err, treefs.ErrRefMoved) {
			return err
		}
	}
	return treefs.ErrRefMoved
}

func TestSignalRecordJSONShape(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()

	_, path, err := env.Store.EmitSignal("test-y", "verify", map[string]any{"phase": "PASS"})
	if err != nil {
		t.Fatalf("EmitSignal: %v", err)
	}
	env.CommitIntent("signal test-y verify " + path)

	data, err := env.Repo.TreeFS().ReadFile("signals/test-y/0001.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var rec issue.SignalRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rec.Seq != 1 || rec.Type != "verify" || rec.Ticket != "test-y" || rec.Payload["phase"] != "PASS" {
		t.Fatalf("record = %#v", rec)
	}
}
