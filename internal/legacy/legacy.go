// Package legacy keeps paths renamed by ADR-26 (supervisor → admiral)
// working for binaries and sessions that still use the old name.
package legacy

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/sturlabragason/fleet/internal/logx"
)

var warned sync.Map // newPath → struct{}: "both real" is logged once

// MoveDir returns the directory to use for a path renamed from oldPath to
// newPath (same parent):
//   - old is a real dir, new missing: rename old → new and leave old as a
//     relative symlink to new, so old binaries keep using the same files;
//     if the rename fails, return old.
//   - new exists, old missing: add the old → new symlink.
//   - both real dirs: new wins, warn once.
//   - neither: create nothing (callers use MkdirAll, which links).
//
// Concurrent callers are fine: a rename lost to another process or a
// symlink that already exists is not an error.
func MoveDir(newPath, oldPath string) string {
	oldSt, oldErr := os.Lstat(oldPath)
	oldReal := oldErr == nil && oldSt.IsDir()
	_, newErr := os.Lstat(newPath)
	newExists := newErr == nil
	switch {
	case oldReal && newExists:
		if _, dup := warned.LoadOrStore(newPath, struct{}{}); !dup {
			logx.For("legacy").Warn("old and new dirs both exist; using new", "new", newPath, "old", oldPath)
		}
	case oldReal:
		if err := os.Rename(oldPath, newPath); err != nil {
			if _, e := os.Lstat(newPath); e == nil {
				return newPath // another process moved it first
			}
			logx.For("legacy").Warn("can't move old dir; using it", "old", oldPath, "new", newPath, "err", err)
			return oldPath
		}
		link(newPath, oldPath)
	case newExists && os.IsNotExist(oldErr):
		link(newPath, oldPath)
	}
	return newPath
}

// MkdirAll creates the directory MoveDir picks and, when that is newPath,
// the old → new symlink if the old path is absent. It returns the dir.
func MkdirAll(newPath, oldPath string) (string, error) {
	dir := MoveDir(newPath, oldPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dir, err
	}
	if dir == newPath {
		if _, err := os.Lstat(oldPath); os.IsNotExist(err) {
			link(newPath, oldPath)
		}
	}
	return dir, nil
}

// Resolve is MoveDir without side effects, for readers that don't hold the
// lock the move needs: newPath if it exists, else oldPath if that is a real
// dir (not moved yet, so nothing in it is missed), else newPath.
func Resolve(newPath, oldPath string) string {
	if _, err := os.Lstat(newPath); err == nil {
		return newPath
	}
	if st, err := os.Lstat(oldPath); err == nil && st.IsDir() {
		return oldPath
	}
	return newPath
}

// link points oldPath at newPath's base name (relative: same parent). If an
// old binary recreated oldPath as a real dir in the meantime, an empty one is
// replaced by the link; a non-empty one is left alone with a warning.
func link(newPath, oldPath string) {
	err := os.Symlink(filepath.Base(newPath), oldPath)
	if err == nil {
		return
	}
	if os.IsExist(err) {
		st, lerr := os.Lstat(oldPath)
		if lerr != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return // a link (ours or another process's) or gone again
		}
		if os.Remove(oldPath) == nil { // fails unless empty
			if err = os.Symlink(filepath.Base(newPath), oldPath); err == nil || os.IsExist(err) {
				return
			}
		} else {
			logx.For("legacy").Warn("old dir was recreated with files in it; not linked", "old", oldPath, "new", newPath)
			return
		}
	}
	logx.For("legacy").Warn("can't link old dir", "old", oldPath, "err", err)
}
