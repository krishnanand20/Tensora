//go:build darwin || linux

package hostlock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Acquire takes the lock at path without waiting: ErrHeld if another process has it. release
// gives it up.
func Acquire(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // the path comes from operator config
	if err != nil {
		return nil, fmt.Errorf("agent lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // a file descriptor fits in an int
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrHeld
		}
		return nil, fmt.Errorf("agent lock: %w", err)
	}
	return func() { _ = f.Close() }, nil // closing the file releases the lock
}
