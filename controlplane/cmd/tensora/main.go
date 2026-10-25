// Command tensora runs the Tensora control plane. One binary, one image; the subcommand
// picks the role. See `tensora help`.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tensora/tensora/controlplane/internal/api"
	"github.com/tensora/tensora/controlplane/internal/auth"
	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/cluster"
	"github.com/tensora/tensora/controlplane/internal/cluster/fake"
	"github.com/tensora/tensora/controlplane/internal/cluster/kube"
	"github.com/tensora/tensora/controlplane/internal/config"
	"github.com/tensora/tensora/controlplane/internal/controller"
	"github.com/tensora/tensora/controlplane/internal/devseed"
	"github.com/tensora/tensora/controlplane/internal/metering"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/store"
	"github.com/tensora/tensora/controlplane/internal/store/memstore"
	"github.com/tensora/tensora/controlplane/internal/store/pgstore"
	"github.com/tensora/tensora/controlplane/internal/store/sqlitestore"
)

const usage = `tensora — free, self-hosted GPU cloud

Usage: tensora <command> [arguments]

Server commands:
  api         serve the REST API
  controller  reconcile workloads with the runtime (Kubernetes today; node agents planned)
  metering    usage accounting: turn workload events into hourly usage records (no money)
  migrate     apply database migrations to TENSORA_DATABASE_URL (Postgres or a SQLite file;
              a SQLite file is also migrated whenever it's opened)
  seed        create dev tenants and quotas (TENSORA_ENV=dev only)
  setup -email you@example.com [-tenant home] [-password-stdin]
              create the first account, the machine's owner, for TENSORA_AUTH_MODE=local; asks for
              the password without echoing it (or reads one line from stdin)
  dev         api + controller + metering in one process with seeded data; uses an in-memory
              store unless TENSORA_DATABASE_URL is set (postgres://… or sqlite:<path>)

Client commands (talk to a running API):
  workloads [-all]      list your workloads (-all includes deleted ones)
  events <id> [-json]   show a workload's complete timeline: every state change and note, when, and why
  export <id> [-o file] [-f]
                        save everything recorded about a workload as one JSON file, readable by
                        you only (default tensora-<name>-<id>.json; -o - writes to stdout;
                        -f replaces an existing file)

Server configuration: TENSORA_* environment variables (see .env.example). Set TENSORA_LOG_LEVEL=debug
for detailed logs and TENSORA_LOG_FILE=path to keep a complete copy on disk.
Client configuration: TENSORA_API_URL (default http://127.0.0.1:8080), and TENSORA_TOKEN (bearer)
or TENSORA_DEV_TENANT (dev mode). A token is never sent over plain http:// to another machine
unless TENSORA_INSECURE_HTTP=true.
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch {
	case isClientCommand(cmd):
		err = runClient(ctx, cmd, args, os.Stdout)
	case cmd == "setup":
		err = runSetup(ctx, args, os.Stdin, os.Stdout)
	case len(args) > 0:
		err = fmt.Errorf("%s takes no arguments (see tensora help)", cmd)
	default:
		err = run(ctx, cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tensora:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log, closeLog, err := newLogger(cfg)
	if err != nil {
		return err
	}
	defer closeLog()

	if cmd == "migrate" {
		db, err := cfg.Database()
		if err != nil {
			return err
		}
		var n int
		switch db.Driver {
		case "postgres":
			n, err = pgstore.Migrate(ctx, db.DSN)
		case "sqlite":
			log.Info("store: sqlite", "path", db.DSN)
			n, err = sqlitestore.Migrate(ctx, db.DSN)
		default:
			return errors.New("TENSORA_DATABASE_URL is required")
		}
		log.Info("migrations applied", "count", n)
		return err
	}

	cat, err := catalog.Load(cfg.CatalogPath)
	if err != nil {
		return err
	}
	st, err := openStore(ctx, cfg, log, cmd == "dev")
	if err != nil {
		return err
	}
	defer st.Close()

	switch cmd {
	case "api":
		return serveAPI(ctx, cfg, log, st, cat)
	case "controller":
		ctrl, err := newController(cfg, log, st, cat)
		if err != nil {
			return err
		}
		return ctrl.Run(ctx, cfg.ControllerInterval)
	case "metering":
		return metering.New(st, log, nil).Run(ctx, cfg.MeteringInterval)
	case "seed":
		if cfg.Env != "dev" {
			return errors.New("seed only runs with TENSORA_ENV=dev")
		}
		return devseed.Seed(ctx, st, time.Now())
	case "dev":
		if cfg.Env != "dev" {
			return errors.New("dev only runs with TENSORA_ENV=dev")
		}
		if err := devseed.Seed(ctx, st, time.Now()); err != nil {
			return err
		}
		ctrl, err := newController(cfg, log, st, cat)
		if err != nil {
			return err
		}
		log.Info("dev stack: seeded tenants acme, globex (untrusted) and internal (trusted); use header X-Dev-Tenant")
		return runAll(ctx,
			func(ctx context.Context) error { return serveAPI(ctx, cfg, log, st, cat) },
			func(ctx context.Context) error { return ctrl.Run(ctx, cfg.ControllerInterval) },
			func(ctx context.Context) error { return metering.New(st, log, nil).Run(ctx, cfg.MeteringInterval) },
		)
	}
	return fmt.Errorf("unknown command %q (see tensora help)", cmd)
}

// newLogger writes to stderr and, if TENSORA_LOG_FILE is set, a complete copy to that file
// (appended, readable by the owner only). Nothing is sampled or dropped.
func newLogger(cfg config.Config) (*slog.Logger, func(), error) {
	var out io.Writer = os.Stderr
	closeFn := func() {}
	if cfg.LogFile != "" {
		f, err := openLogFile(cfg.LogFile)
		if err != nil {
			return nil, nil, fmt.Errorf("open TENSORA_LOG_FILE: %w", err)
		}
		out = io.MultiWriter(os.Stderr, f)
		closeFn = func() { _ = f.Close() }
	}
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.Env == "dev" {
		return slog.New(slog.NewTextHandler(out, opts)), closeFn, nil
	}
	return slog.New(slog.NewJSONHandler(out, opts)), closeFn, nil
}

// openLogFile appends to path, which must be readable by its owner only. It never changes a file's
// permissions, so a link swapped in by someone else can't redirect a chmod. A new file is created
// 0600; an existing file that others can read is refused with the fix. Symlinks, devices, and a
// file replaced while it was being opened are refused too.
func openLogFile(path string) (*os.File, error) {
	path = filepath.Clean(path)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file, not a symlink or device", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err == nil {
		var now os.FileInfo
		now, err = os.Lstat(path)
		switch {
		case err != nil:
		case !now.Mode().IsRegular() || !os.SameFile(opened, now):
			err = fmt.Errorf("%s changed while it was being opened", path)
		case opened.Mode().Perm()&0o077 != 0:
			err = fmt.Errorf("%s can be read by other users (mode %v); run: chmod 600 %s", path, opened.Mode().Perm(), path)
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// listensOnlyOnLoopback reports whether addr ("127.0.0.1:8080", ":8080") accepts only this machine.
func listensOnlyOnLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.IsLoopback()
}

func openStore(ctx context.Context, cfg config.Config, log *slog.Logger, allowMemory bool) (store.Store, error) {
	db, err := cfg.Database()
	if err != nil {
		return nil, err
	}
	switch db.Driver {
	case "postgres":
		log.Info("store: postgres") // never log the URL: it can hold a password
		return pgstore.Open(ctx, db.DSN)
	case "sqlite":
		log.Info("store: sqlite", "path", db.DSN)
		return sqlitestore.Open(ctx, db.DSN) // applies pending migrations itself
	}
	if !allowMemory {
		return nil, errors.New("TENSORA_DATABASE_URL is required (only `tensora dev` can run in-memory)")
	}
	return memstore.New(), nil
}

func newController(cfg config.Config, log *slog.Logger, st store.Store, cat *catalog.Catalog) (*controller.Controller, error) {
	// A controller for a region users can't create workloads in would leave everything pending.
	if _, ok := cat.Region(cfg.Region); !ok {
		return nil, fmt.Errorf("TENSORA_REGION %q isn't in the catalog's regions", cfg.Region)
	}
	var backend cluster.Backend
	switch cfg.ClusterBackend {
	case "kubernetes":
		b, err := kube.New(cat)
		if err != nil {
			return nil, err
		}
		backend = b
	default:
		backend = fake.New(cfg.FakeCapacity)
	}
	return controller.New(st, backend, cat, controller.Options{
		Region:                cfg.Region,
		StorageClass:          cfg.StorageClass,
		ImagesNamespace:       cfg.ImagesNamespace,
		UntrustedRuntimeClass: cfg.UntrustedRuntimeClass,
		ProvisionTimeout:      cfg.ProvisionTimeout,
	}, log.With("component", "controller"), nil), nil
}

func serveAPI(ctx context.Context, cfg config.Config, log *slog.Logger, st store.Store, cat *catalog.Catalog) error {
	svc := service.New(st, cat, nil)
	var authn auth.Authenticator
	switch cfg.AuthMode {
	case "oidc":
		o, err := auth.NewOIDC(ctx, cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCTenantClaim)
		if err != nil {
			return err
		}
		authn = o
	case "local":
		authn = auth.NewLocal(svc, auth.Transport{TrustedProxies: cfg.TrustedProxies, AllowPlainHTTP: cfg.InsecureHTTP})
		if n, err := countUsers(ctx, st); err == nil && n == 0 {
			log.Warn("local auth: no accounts yet; create the owner with `tensora setup`")
		}
		if !listensOnlyOnLoopback(cfg.HTTPAddr) && !cfg.InsecureHTTP && len(cfg.TrustedProxies) == 0 {
			log.Warn("local auth: listening beyond this machine, but sign-in from other machines needs HTTPS "+
				"through a trusted proxy (TENSORA_TRUSTED_PROXIES), or TENSORA_INSECURE_HTTP=true on a network you trust",
				"addr", cfg.HTTPAddr)
		}
	default:
		authn = auth.Dev{}
		log.Warn("dev auth enabled: the X-Dev-Tenant header is trusted")
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(svc, authn, log.With("component", "api"), st.Ping).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// runAll runs fns concurrently; when any returns, the rest are cancelled. Returns the first error.
func runAll(ctx context.Context, fns ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, len(fns))
	for _, fn := range fns {
		go func() { errc <- fn(ctx) }()
	}
	var first error
	for range fns {
		if err := <-errc; err != nil && first == nil {
			first = err
		}
		cancel()
	}
	return first
}
