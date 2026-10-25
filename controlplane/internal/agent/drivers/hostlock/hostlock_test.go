package hostlock

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOneHolderAtATime(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the agent runs on macOS and Linux")
	}
	path := filepath.Join(t.TempDir(), "agent.lock")
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path); !errors.Is(err, ErrHeld) {
		t.Errorf("a second holder: %v; want ErrHeld", err)
	}
	release()
	again, err := Acquire(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}
