package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensora/tensora/controlplane/internal/config"
	"github.com/tensora/tensora/controlplane/internal/store/memstore"
	"github.com/tensora/tensora/controlplane/internal/testutil"
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

// A controller for a region the catalog doesn't offer would leave every workload pending.
func TestControllerRegionMustBeInTheCatalog(t *testing.T) {
	cfg := config.Config{Region: "eu-fra-1", ClusterBackend: "fake"}
	_, err := newController(cfg, slog.New(slog.DiscardHandler), memstore.New(), testutil.Catalog(t))
	if err == nil || !strings.Contains(err.Error(), "isn't in the catalog's regions") {
		t.Fatalf("err = %v", err)
	}
	cfg.Region = "local"
	if _, err := newController(cfg, slog.New(slog.DiscardHandler), memstore.New(), testutil.Catalog(t)); err != nil {
		t.Fatal(err)
	}
}

// tensora setup creates the owner once, on the store itself; a second run is refused.
func TestSetupCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TENSORA_DATABASE_URL", "sqlite:"+filepath.Join(dir, "tensora.db"))
	t.Setenv("TENSORA_CATALOG_PATH", testutilCatalogPath(t))
	t.Setenv("TENSORA_AUTH_MODE", "local")
	ctx := context.Background()

	var out bytes.Buffer
	in := strings.NewReader("correct horse battery staple\n")
	if err := runSetup(ctx, []string{"-email", "me@example.com", "-password-stdin"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Created the owner account me@example.com") || strings.Contains(out.String(), "staple") {
		t.Errorf("output = %q", out.String())
	}
	err := runSetup(ctx, []string{"-email", "other@example.com", "-password-stdin"}, strings.NewReader("another long passphrase\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second setup: %v; want it refused", err)
	}
	if err := runSetup(ctx, []string{"-email", "x@example.com"}, strings.NewReader(""), &out); err == nil {
		t.Error("without a terminal or -password-stdin there's no safe way to read a password")
	}

	// The way back in for a forgotten password.
	out.Reset()
	if err := runSetup(ctx, []string{"-reset-password", "-email", "Me@Example.com", "-password-stdin"},
		strings.NewReader("a brand new passphrase\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Set a new password for me@example.com") || strings.Contains(out.String(), "passphrase") {
		t.Errorf("output = %q", out.String())
	}
	err = runSetup(ctx, []string{"-reset-password", "-email", "nobody@example.com", "-password-stdin"},
		strings.NewReader("a brand new passphrase\n"), &out)
	if err == nil {
		t.Error("reset for an unknown email succeeded")
	}
}

func testutilCatalogPath(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		p := filepath.Join(dir, "images", "catalog.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("images/catalog.yaml not found")
		}
		dir = parent
	}
}
