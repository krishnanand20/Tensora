// Package hostlock makes sure only one agent acts for a machine at a time, whatever the region
// lease's timing (a pause, a clock jump, a slow shutdown): an exclusive OS lock on a file in the
// agent's state directory, released by the OS if the process dies.
package hostlock

import "errors"

// ErrHeld means another process holds the lock.
var ErrHeld = errors.New("another agent is running for this machine")
