package treefs

import (
	"errors"
	"strings"

	"github.com/jallum/go-git/v5/plumbing"
	"github.com/jallum/go-git/v5/storage/filesystem/dotgit"
)

// IsObjectStoreRace reports whether err looks like go-git losing a race with
// a concurrent git repack. A repack (git's auto-maintenance, or any git
// command in another worktree of the same repository) can delete a pack,
// prune a loose object it just packed, or remove an emptied loose-object
// fan-out directory between go-git listing the object store and using it.
// The data is intact; a fresh go-git handle sees it. See issue #149.
func IsObjectStoreRace(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, plumbing.ErrObjectNotFound) || errors.Is(err, dotgit.ErrPackfileNotFound) {
		return true
	}
	// go-git writes a loose object to objects/pack/tmp_obj_N, then renames it
	// into objects/xx/. A concurrent prune can remove objects/xx/ in between.
	msg := err.Error()
	return strings.Contains(msg, "rename ") && strings.Contains(msg, "tmp_obj_")
}
