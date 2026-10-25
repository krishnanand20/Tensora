//go:build !darwin && !linux

package hostlock

// Acquire is a no-op where the agent doesn't run.
func Acquire(string) (func(), error) { return func() {}, nil }
