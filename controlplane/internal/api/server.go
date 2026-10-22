// Package api is the public REST API (/v1). Handlers are thin: decode, call the service,
// encode. They never touch the store or the cluster directly.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/auth"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
)

const maxBodyBytes = 1 << 20

type Server struct {
	svc   *service.Service
	auth  auth.Authenticator
	log   *slog.Logger
	ready func(context.Context) error
}

func New(svc *service.Service, a auth.Authenticator, log *slog.Logger, ready func(context.Context) error) *Server {
	return &Server{svc: svc, auth: a, log: log, ready: ready}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", s.readyz)

	mux.Handle("GET /v1/catalog", s.authed(s.getCatalog))
	mux.Handle("GET /v1/workloads", s.authed(s.listWorkloads))
	mux.Handle("POST /v1/workloads", s.authed(s.createWorkload))
	mux.Handle("GET /v1/workloads/{id}", s.authed(s.getWorkload))
	mux.Handle("POST /v1/workloads/{id}/stop", s.authed(s.workloadAction(s.svc.StopWorkload)))
	mux.Handle("POST /v1/workloads/{id}/start", s.authed(s.workloadAction(s.svc.StartWorkload)))
	mux.Handle("DELETE /v1/workloads/{id}", s.authed(s.workloadAction(s.svc.DeleteWorkload)))

	return http.MaxBytesHandler(s.logRequests(mux), maxBodyBytes)
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, p domain.Principal) error

func (s *Server) authed(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.auth.Authenticate(r)
		if err == nil {
			err = h(w, r, p)
		}
		if err != nil {
			s.writeError(w, r, err)
		}
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.ready(ctx); err != nil {
		s.log.WarnContext(ctx, "not ready", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) getCatalog(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
	classes, images, err := s.svc.VisibleCatalog(r.Context(), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toCatalogView(classes, images))
	return nil
}

func (s *Server) listWorkloads(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
	ws, err := s.svc.ListWorkloads(r.Context(), p)
	if err != nil {
		return err
	}
	items := make([]workloadView, len(ws))
	for i, wl := range ws {
		items[i] = toWorkloadView(wl)
	}
	writeJSON(w, http.StatusOK, listView[workloadView]{Items: items})
	return nil
}

func (s *Server) createWorkload(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
	var in service.CreateWorkloadInput
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	wl, err := s.svc.CreateWorkload(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, toWorkloadView(wl))
	return nil
}

func (s *Server) getWorkload(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	wl, err := s.svc.GetWorkload(r.Context(), p, id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toWorkloadView(wl))
	return nil
}

// workloadAction adapts stop/start/delete. They return 202: the controller does the work.
func (s *Server) workloadAction(fn func(context.Context, domain.Principal, uuid.UUID) (domain.Workload, error)) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		id, err := pathID(r)
		if err != nil {
			return err
		}
		wl, err := fn(r.Context(), p, id)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, toWorkloadView(wl))
		return nil
	}
}

func pathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil, domain.Errorf(domain.CodeNotFound, "workload not found")
	}
	return id, nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if r.Pattern == "GET /healthz" || r.Pattern == "GET /readyz" {
			level = slog.LevelDebug
		}
		s.log.Log(r.Context(), level, "http request",
			"method", r.Method, "route", r.Pattern, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}
