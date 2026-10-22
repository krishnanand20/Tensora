// Command tensora runs the Tensora control plane. One binary, one image; the subcommand
// picks the role. See `tensora help`.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
)

const usage = `tensora — Tensora control plane

Usage: tensora <command>

Commands:
  api         serve the REST API
  controller  reconcile workloads between Postgres and Kubernetes
  metering    turn workload events into hourly usage records
  migrate     apply database migrations to TENSORA_DATABASE_URL
  seed        create dev tenants and quotas (TENSORA_ENV=dev only)
  dev         api + controller + metering in one process with seeded data;
              uses an in-memory store unless TENSORA_DATABASE_URL is set

Configuration is read from TENSORA_* environment variables (see .env.example).
`

func main() {
	if len(os.Args) != 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "tensora:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)

	if cmd == "migrate" {
		if cfg.DatabaseURL == "" {
			return errors.New("TENSORA_DATABASE_URL is required")
		}
		n, err := pgstore.Migrate(ctx, cfg.DatabaseURL)
		log.Info("migrations applied", "count", n)
		return err
	}

	cat, err := catalog.Load(cfg.CatalogPath)
	if err != nil {
		return err
	}
	st, err := openStore(ctx, cfg, cmd == "dev")
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

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.Env == "dev" {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

func openStore(ctx context.Context, cfg config.Config, allowMemory bool) (store.Store, error) {
	if cfg.DatabaseURL != "" {
		return pgstore.Open(ctx, cfg.DatabaseURL)
	}
	if !allowMemory {
		return nil, errors.New("TENSORA_DATABASE_URL is required (only `tensora dev` can run in-memory)")
	}
	return memstore.New(), nil
}

func newController(cfg config.Config, log *slog.Logger, st store.Store, cat *catalog.Catalog) (*controller.Controller, error) {
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
		StorageClass:          cfg.StorageClass,
		ImagesNamespace:       cfg.ImagesNamespace,
		UntrustedRuntimeClass: cfg.UntrustedRuntimeClass,
		ProvisionTimeout:      cfg.ProvisionTimeout,
	}, log.With("component", "controller"), nil), nil
}

func serveAPI(ctx context.Context, cfg config.Config, log *slog.Logger, st store.Store, cat *catalog.Catalog) error {
	var authn auth.Authenticator = auth.Dev{}
	if cfg.AuthMode == "oidc" {
		o, err := auth.NewOIDC(ctx, cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCTenantClaim)
		if err != nil {
			return err
		}
		authn = o
	} else {
		log.Warn("dev auth enabled: the X-Dev-Tenant header is trusted")
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(service.New(st, cat, nil), authn, log.With("component", "api"), st.Ping).Handler(),
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
