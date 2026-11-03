package repo

import (
	"os"
	"path/filepath"
	"time"

	"github.com/jallum/beadwork/internal/treefs"
)

// IsObjectStoreRace reports whether err comes from go-git losing a race with
// a concurrent git repack. See treefs.IsObjectStoreRace.
func IsObjectStoreRace(err error) bool { return treefs.IsObjectStoreRace(err) }

// maintenanceLockWait caps how long WaitForMaintenance waits for a running
// git maintenance to finish. A geometric repack of a large repository can
// take tens of seconds.
const maintenanceLockWait = 60 * time.Second

// WaitForMaintenance blocks while git maintenance holds its lock on this
// repository's object directory, up to maintenanceLockWait. Git takes the
// lock for every `git maintenance run`, including the detached --auto run
// that fetch, commit and merge start, so its absence means no repack is
// rewriting packs.
func (r *Repo) WaitForMaintenance() {
	waitForMaintenance(r.GitDir)
}

func waitForMaintenance(gitDir string) {
	lock := filepath.Join(gitDir, "objects", "maintenance.lock")
	deadline := time.Now().Add(maintenanceLockWait)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(lock); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// RetryObjectStoreRace runs op, and when it fails with an object-store race,
// waits for any running maintenance, reopens the go-git handle and runs op
// again, up to attempts times in total. op must re-derive its state from the
// repository on every call.
func (r *Repo) RetryObjectStoreRace(attempts int, op func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			raceBackoff(r.GitDir, i)
			if _, rerr := r.Reopen(); rerr != nil && !IsObjectStoreRace(rerr) {
				return rerr
			}
		}
		if err = op(); err == nil || !IsObjectStoreRace(err) {
			return err
		}
	}
	return err
}

// raceBackoff waits before retry attempt i (1-based): a short linear sleep,
// then for any running maintenance to release its lock.
func raceBackoff(gitDir string, attempt int) {
	time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
	waitForMaintenance(gitDir)
}

// raceAttempts bounds retries of an operation that lost an object-store race.
const raceAttempts = 5
