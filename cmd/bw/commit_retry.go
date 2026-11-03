package main

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
	"github.com/jallum/beadwork/internal/treefs"
)

// commitMaxRetries bounds how many times mutating commands retry a commit
// after losing the CAS race against a concurrent writer. Set generously
// because in real-world use (multiple agents, background sync) the race
// can be sustained over many attempts; 3 turned out to be too few.
const commitMaxRetries = 12

// commitBackoffBase is the base delay between retry attempts. The actual
// delay is roughly base * 2^(attempt-1) with jitter, capped at
// commitBackoffMax. Backoff desynchronizes colliding writers so the
// retry storm dissipates.
const commitBackoffBase = 5 * time.Millisecond

// commitBackoffMax caps the per-attempt delay so a stuck retry loop
// finishes in bounded time.
const commitBackoffMax = 250 * time.Millisecond

// commitWithRetry runs apply, then store.Commit(intent), retrying up to
// maxRetries times when the commit fails with treefs.ErrRefMoved. Between
// attempts the store fully reopens its TreeFS (fresh go-git handle, no
// cached state) so apply observes the latest ref state, then waits a
// jittered backoff before re-running.
//
// apply must re-derive all state from the store on every invocation —
// values captured from a previous attempt are stale after reopen and
// must not be reused.
func commitWithRetry(store *issue.Store, maxRetries int, apply func() (intent string, err error)) error {
	var commitErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			sleepBackoff(attempt)
			if err := store.ReopenFS(); err != nil {
				if treefs.IsObjectStoreRace(err) {
					commitErr = err
					waitForMaintenance(store)
					continue
				}
				return fmt.Errorf("reopen after conflict: %w", err)
			}
		}
		intent, aerr := apply()
		if aerr != nil {
			if !treefs.IsObjectStoreRace(aerr) {
				return aerr
			}
			commitErr = aerr
			waitForMaintenance(store)
			continue
		}
		commitErr = store.Commit(intent)
		if commitErr == nil {
			return nil
		}
		if treefs.IsObjectStoreRace(commitErr) {
			// A concurrent repack pruned an object or its fan-out directory
			// under go-git; the next attempt reopens and writes it again.
			waitForMaintenance(store)
			continue
		}
		if !errors.Is(commitErr, treefs.ErrRefMoved) {
			return fmt.Errorf("commit failed: %w", commitErr)
		}
	}
	return fmt.Errorf("commit failed after %d attempts: %w", maxRetries, commitErr)
}

// sleepBackoff sleeps for an exponentially-growing, jittered duration.
// attempt is 1-based.
func sleepBackoff(attempt int) {
	d := commitBackoffBase << (attempt - 1)
	if d > commitBackoffMax || d <= 0 {
		d = commitBackoffMax
	}
	// Full jitter: pick a duration in [0, d). Avoids thundering-herd
	// retries when several writers collide at the same instant.
	jittered := time.Duration(rand.Int63n(int64(d) + 1))
	time.Sleep(jittered)
}

// waitForMaintenance blocks while git maintenance is repacking the store's
// repository, so the retry after an object-store race reads settled packs.
func waitForMaintenance(store *issue.Store) {
	if r, ok := store.Committer.(*repo.Repo); ok {
		r.WaitForMaintenance()
	}
}
