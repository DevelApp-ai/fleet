package supervisor

import (
	"os"
	"path/filepath"
	"syscall"
)

// Lock takes an exclusive lock on supervisor/reset.lock so concurrent resets
// (M-R, overview R, fleet_handoff) run one at a time. Call unlock when done.
func Lock(home string) (unlock func(), err error) {
	if err := os.MkdirAll(Dir(home), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(Dir(home), "reset.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
