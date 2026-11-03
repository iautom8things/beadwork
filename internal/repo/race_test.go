package repo_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jallum/beadwork/internal/repo"
	"github.com/jallum/beadwork/internal/testutil"
	"github.com/jallum/go-git/v5/plumbing"
	"github.com/jallum/go-git/v5/storage/filesystem/dotgit"
)

func TestIsObjectStoreRace(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("walk local commits: %w", plumbing.ErrObjectNotFound), true},
		{fmt.Errorf("read local tree: %w", dotgit.ErrPackfileNotFound), true},
		{errors.New("build tree: rename .git/objects/pack/tmp_obj_1 .git/objects/e6/9d: invalid argument"), true},
		{errors.New("issue t-abc not found"), false},
		{plumbing.ErrReferenceNotFound, false},
	}
	for _, c := range cases {
		if got := repo.IsObjectStoreRace(c.err); got != c.want {
			t.Errorf("IsObjectStoreRace(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestRetryObjectStoreRaceReopensAndRetries(t *testing.T) {
	env := testutil.NewEnv(t)
	defer env.Cleanup()

	calls := 0
	err := env.Repo.RetryObjectStoreRace(3, func() error {
		calls++
		if calls == 1 {
			return fmt.Errorf("walk local commits: %w", plumbing.ErrObjectNotFound)
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err = %v, calls = %d; want nil after 2 calls", err, calls)
	}

	calls = 0
	other := errors.New("push rejected")
	if err := env.Repo.RetryObjectStoreRace(3, func() error { calls++; return other }); err != other || calls != 1 {
		t.Fatalf("err = %v, calls = %d; want the non-race error after 1 call", err, calls)
	}
}
