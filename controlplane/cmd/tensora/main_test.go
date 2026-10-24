package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensora/tensora/controlplane/internal/config"
)

func TestLogFileGetsACompleteCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tensora.log")
	log, closeLog, err := newLogger(config.Config{Env: "production", LogLevel: slog.LevelDebug, LogFile: path})
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("admission check", "workload", "w1")
	log.Info("workload transitioned", "to", "running")
	closeLog()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"msg":"admission check"`, `"msg":"workload transitioned"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log file is missing %s:\n%s", want, data)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("log file mode = %v; logs are readable by their owner only", info.Mode().Perm())
	}
}

func TestLogFileMustBePrivateAndSymlinksAreRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	// A file others can read is refused, with the fix, rather than silently chmodded.
	if _, _, err := newLogger(config.Config{Env: "production", LogFile: path}); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("world-readable log file: err = %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	_, closeLog, err := newLogger(config.Config{Env: "production", LogFile: path})
	if err != nil {
		t.Fatal(err)
	}
	closeLog()

	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newLogger(config.Config{Env: "production", LogFile: link}); err == nil {
		t.Error("a symlinked TENSORA_LOG_FILE must be refused")
	}
}
