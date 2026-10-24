// Package config loads process configuration from TENSORA_* environment variables.
// .env.example documents every variable.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env      string // dev | staging | production
	HTTPAddr string
	LogLevel slog.Level
	// LogFile, if set, receives a complete copy of every log line (in addition to stderr).
	LogFile string

	// DatabaseURL selects the store: postgres://… or postgresql://… for Postgres, sqlite:<path> for a
	// SQLite file (Solo and Lab, ADR-0012), or empty for the in-memory store (dev only).
	DatabaseURL string
	CatalogPath string

	AuthMode        string // dev | oidc
	OIDCIssuer      string
	OIDCAudience    string
	OIDCTenantClaim string

	ClusterBackend        string // fake | kubernetes
	FakeCapacity          map[string]int
	UntrustedRuntimeClass string
	StorageClass          string
	ImagesNamespace       string

	ControllerInterval time.Duration
	MeteringInterval   time.Duration
	ProvisionTimeout   time.Duration
}

func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv builds a Config from getenv, applying defaults and validating the result.
func FromEnv(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv("TENSORA_" + key)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Env:                   get("ENV", "dev"),
		HTTPAddr:              get("HTTP_ADDR", "127.0.0.1:8080"),
		LogFile:               get("LOG_FILE", ""),
		DatabaseURL:           get("DATABASE_URL", ""),
		CatalogPath:           get("CATALOG_PATH", "../images/catalog.yaml"),
		AuthMode:              get("AUTH_MODE", "dev"),
		OIDCIssuer:            get("OIDC_ISSUER", ""),
		OIDCAudience:          get("OIDC_AUDIENCE", "tensora-api"),
		OIDCTenantClaim:       get("OIDC_TENANT_CLAIM", "tensora_tenant"),
		ClusterBackend:        get("CLUSTER_BACKEND", "fake"),
		UntrustedRuntimeClass: get("UNTRUSTED_RUNTIME_CLASS", "kata-qemu-nvidia-gpu"),
		StorageClass:          get("STORAGE_CLASS", "tensora-block"),
		ImagesNamespace:       get("IMAGES_NAMESPACE", "tensora-images"),
	}

	var errs []error
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("TENSORA_LOG_LEVEL: %w", err))
	}
	var err error
	if c.FakeCapacity, err = parseCapacity(get("FAKE_CAPACITY", "l40s-48gb=4,a100-80gb=2")); err != nil {
		errs = append(errs, fmt.Errorf("TENSORA_FAKE_CAPACITY: %w", err))
	}
	for _, d := range []struct {
		key, def string
		dst      *time.Duration
	}{
		{"CONTROLLER_INTERVAL", "5s", &c.ControllerInterval},
		{"METERING_INTERVAL", "5m", &c.MeteringInterval},
		{"PROVISION_TIMEOUT", "30m", &c.ProvisionTimeout},
	} {
		if *d.dst, err = time.ParseDuration(get(d.key, d.def)); err != nil {
			errs = append(errs, fmt.Errorf("TENSORA_%s: %w", d.key, err))
		}
	}
	errs = append(errs, c.validate())
	return c, errors.Join(errs...)
}

func (c Config) validate() error {
	var errs []error
	switch c.Env {
	case "dev", "staging", "production":
	default:
		errs = append(errs, fmt.Errorf("TENSORA_ENV must be dev, staging, or production, got %q", c.Env))
	}
	switch c.AuthMode {
	case "dev":
		if c.Env != "dev" {
			errs = append(errs, errors.New("TENSORA_AUTH_MODE=dev trusts request headers and is only allowed when TENSORA_ENV=dev"))
		}
	case "oidc":
		if c.OIDCIssuer == "" {
			errs = append(errs, errors.New("TENSORA_OIDC_ISSUER is required when TENSORA_AUTH_MODE=oidc"))
		}
	default:
		errs = append(errs, fmt.Errorf("TENSORA_AUTH_MODE must be dev or oidc, got %q", c.AuthMode))
	}
	switch c.ClusterBackend {
	case "fake":
		if c.Env == "production" {
			errs = append(errs, errors.New("TENSORA_CLUSTER_BACKEND=fake is not allowed in production"))
		}
	case "kubernetes":
	default:
		errs = append(errs, fmt.Errorf("TENSORA_CLUSTER_BACKEND must be fake or kubernetes, got %q", c.ClusterBackend))
	}
	if c.Env != "dev" && c.DatabaseURL == "" {
		errs = append(errs, errors.New("TENSORA_DATABASE_URL is required outside dev"))
	}
	if _, err := c.Database(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Database is the store TENSORA_DATABASE_URL selects.
type Database struct {
	Driver string // "postgres", "sqlite", or "" for the in-memory store (dev only)
	DSN    string // the Postgres URL, or the SQLite file path
}

// Database parses DatabaseURL: postgres://… or postgresql://…, sqlite:<path> (sqlite:///abs/path
// also works), or empty.
func (c Config) Database() (Database, error) {
	u := c.DatabaseURL
	switch {
	case u == "":
		return Database{}, nil
	case strings.HasPrefix(u, "postgres://"), strings.HasPrefix(u, "postgresql://"):
		return Database{Driver: "postgres", DSN: u}, nil
	case strings.HasPrefix(u, "sqlite:"):
		// sqlite:///var/lib/x.db and sqlite:/var/lib/x.db are the same file.
		path := strings.TrimPrefix(strings.TrimPrefix(u, "sqlite:"), "//")
		if path == "" || strings.ContainsRune(path, '?') || path == ":memory:" || strings.HasPrefix(path, "file:") {
			return Database{}, errors.New("TENSORA_DATABASE_URL=sqlite:<path> needs a file path with no '?' (not :memory: or a file: URI)")
		}
		// Resolved once, so every part of the process, and its log, names the same file. Under
		// launchd or systemd the working directory is usually /, so prefer an absolute path.
		abs, err := filepath.Abs(path)
		if err != nil {
			return Database{}, fmt.Errorf("TENSORA_DATABASE_URL: %w", err)
		}
		return Database{Driver: "sqlite", DSN: abs}, nil
	}
	return Database{}, errors.New("TENSORA_DATABASE_URL must be a postgres:// or postgresql:// URL, or sqlite:<path>")
}

// parseCapacity parses "l40s-48gb=4,a100-80gb=2".
func parseCapacity(s string) (map[string]int, error) {
	out := map[string]int{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		class, n, ok := strings.Cut(part, "=")
		count, err := strconv.Atoi(n)
		if !ok || err != nil || count < 0 {
			return nil, fmt.Errorf("want class=count pairs, got %q", part)
		}
		out[class] = count
	}
	return out, nil
}
