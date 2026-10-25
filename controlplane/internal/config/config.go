// Package config loads process configuration from TENSORA_* environment variables.
// .env.example documents every variable.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tensora/tensora/controlplane/internal/domain"
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

	ClusterBackend string // fake | kubernetes | agent
	FakeCapacity   map[string]int
	// MachineOwner is the tenant (slug) that owns this machine when the agent backend runs an agent
	// in-process (Solo): TENSORA_MACHINE_OWNER, home by default, the tenant `tensora setup` makes.
	MachineOwner string
	// AgentDrivers are the in-process agent's drivers (TENSORA_AGENT_DRIVERS, comma-separated).
	// Only sim exists so far, and only in dev; the real drivers are roadmap step 7.
	AgentDrivers []string
	// AgentStateDir keeps this machine's identity for the in-process agent (TENSORA_AGENT_STATE_DIR).
	// Empty means the SQLite database's directory; it's required with Postgres.
	AgentStateDir         string
	UntrustedRuntimeClass string
	StorageClass          string
	ImagesNamespace       string

	ControllerInterval time.Duration
	MeteringInterval   time.Duration
	ProvisionTimeout   time.Duration

	// Region is the one region this process's controller reconciles (ADR-0009); local by default.
	Region domain.Region

	// TrustedProxies are reverse proxies whose X-Forwarded-For and X-Forwarded-Proto are believed
	// (TENSORA_TRUSTED_PROXIES: addresses or CIDR ranges, comma-separated).
	TrustedProxies []netip.Prefix
	// InsecureHTTP allows sign-in and session cookies over plain HTTP from other machines
	// (TENSORA_INSECURE_HTTP=true), for a network the operator trusts.
	InsecureHTTP bool
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
		MachineOwner:          get("MACHINE_OWNER", "home"),
		AgentStateDir:         get("AGENT_STATE_DIR", ""),
		UntrustedRuntimeClass: get("UNTRUSTED_RUNTIME_CLASS", "kata-qemu-nvidia-gpu"),
		StorageClass:          get("STORAGE_CLASS", "tensora-block"),
		ImagesNamespace:       get("IMAGES_NAMESPACE", "tensora-images"),
		Region:                domain.Region(get("REGION", string(domain.RegionLocal))),
	}

	var errs []error
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("TENSORA_LOG_LEVEL: %w", err))
	}
	var err error
	if c.TrustedProxies, err = parsePrefixes(get("TRUSTED_PROXIES", "")); err != nil {
		errs = append(errs, fmt.Errorf("TENSORA_TRUSTED_PROXIES: %w", err))
	}
	if c.InsecureHTTP, err = strconv.ParseBool(get("INSECURE_HTTP", "false")); err != nil {
		errs = append(errs, fmt.Errorf("TENSORA_INSECURE_HTTP must be true or false: %w", err))
	}
	for _, d := range strings.Split(get("AGENT_DRIVERS", ""), ",") {
		if d = strings.TrimSpace(d); d != "" {
			c.AgentDrivers = append(c.AgentDrivers, d)
		}
	}
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
	case "local": // built-in accounts and API tokens (ADR-0012); fine in every environment
	default:
		errs = append(errs, fmt.Errorf("TENSORA_AUTH_MODE must be local, oidc, or dev, got %q", c.AuthMode))
	}
	switch c.ClusterBackend {
	case "fake":
		if c.Env == "production" {
			errs = append(errs, errors.New("TENSORA_CLUSTER_BACKEND=fake is not allowed in production"))
		}
	case "kubernetes":
	case "agent":
		if !domain.ValidSlug(c.MachineOwner) {
			errs = append(errs, fmt.Errorf("TENSORA_MACHINE_OWNER must be a tenant slug, got %q", c.MachineOwner))
		}
		if db, err := c.Database(); err == nil && db.Driver == "postgres" && c.AgentStateDir == "" {
			errs = append(errs, errors.New("TENSORA_AGENT_STATE_DIR is required with the agent backend and Postgres: it keeps this machine's identity"))
		}
		if c.AgentStateDir != "" && !filepath.IsAbs(c.AgentStateDir) {
			errs = append(errs, errors.New("TENSORA_AGENT_STATE_DIR must be an absolute path"))
		}
		for _, d := range c.AgentDrivers {
			switch {
			case d != domain.DriverSim:
				errs = append(errs, fmt.Errorf("TENSORA_AGENT_DRIVERS: unknown driver %q (only sim exists so far)", d))
			case c.Env != "dev":
				errs = append(errs, errors.New("TENSORA_AGENT_DRIVERS=sim runs nothing, so it's only for TENSORA_ENV=dev"))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("TENSORA_CLUSTER_BACKEND must be fake, kubernetes, or agent, got %q", c.ClusterBackend))
	}
	if c.Env != "dev" && c.DatabaseURL == "" {
		errs = append(errs, errors.New("TENSORA_DATABASE_URL is required outside dev"))
	}
	if _, err := c.Database(); err != nil {
		errs = append(errs, err)
	}
	if !c.Region.Valid() {
		errs = append(errs, fmt.Errorf("TENSORA_REGION must be a region ID such as local or eu-fra-1, got %q", c.Region))
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

// parsePrefixes parses comma-separated addresses ("10.0.0.5") and CIDR ranges ("10.0.0.0/24").
func parsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("%q is neither an address nor a CIDR range", part)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
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
