package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/service"
)

// Response shapes. web/src/api/types.ts mirrors these — change both together.
// Views deliberately omit cluster internals (device names, node selectors, object names).

type workloadView struct {
	ID        uuid.UUID           `json:"id"`
	Name      string              `json:"name"`
	Kind      domain.WorkloadKind `json:"kind"`
	State     domain.State        `json:"state"`
	ImageID   string              `json:"image_id"`
	GPUClass  string              `json:"gpu_class"` // the class requested
	GPUCount  int                 `json:"gpu_count"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	// Where it runs and on what (ADR-0009, ADR-0010). placed_class is absent until it's admitted,
	// and differs from gpu_class only after an opt-in fallback.
	Region      domain.Region `json:"region"`
	PlacedClass string        `json:"placed_class,omitempty"`
}

type listView[T any] struct {
	Items []T `json:"items"`
}

type gpuClassView struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	VRAMGB      int    `json:"vram_gb"`
	SupportsVM  bool   `json:"supports_vm"`
}

type imageView struct {
	ID          string              `json:"id"`
	DisplayName string              `json:"display_name"`
	Kind        domain.WorkloadKind `json:"kind"`
	Description string              `json:"description"`
}

type catalogView struct {
	GPUClasses []gpuClassView `json:"gpu_classes"`
	Images     []imageView    `json:"images"`
}

type errorView struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RequestID matches the X-Request-ID header and the operator's log lines for this request.
	RequestID string `json:"request_id,omitempty"`
}

func toWorkloadView(w domain.Workload) workloadView {
	return workloadView{
		ID: w.ID, Name: w.Name, Kind: w.Kind, State: w.State, ImageID: w.ImageID,
		GPUClass: w.GPUClass, GPUCount: w.GPUCount, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
		Region: w.Region, PlacedClass: w.PlacedClass,
	}
}

func toCatalogView(classes []catalog.GPUClass, images []catalog.Image) catalogView {
	v := catalogView{GPUClasses: make([]gpuClassView, len(classes)), Images: make([]imageView, len(images))}
	for i, g := range classes {
		v.GPUClasses[i] = gpuClassView{ID: g.ID, DisplayName: g.DisplayName, VRAMGB: g.VRAMGB, SupportsVM: g.VMDeviceName != ""}
	}
	for i, img := range images {
		v.Images[i] = imageView{ID: img.ID, DisplayName: img.DisplayName, Kind: img.Kind, Description: img.Description}
	}
	return v
}

var statusByCode = map[domain.Code]int{
	domain.CodeInvalid:           http.StatusBadRequest,
	domain.CodeUnauthenticated:   http.StatusUnauthorized,
	domain.CodeForbidden:         http.StatusForbidden,
	domain.CodeNotFound:          http.StatusNotFound,
	domain.CodeConflict:          http.StatusConflict,
	domain.CodeQuotaExceeded:     http.StatusConflict,
	domain.CodeInvalidTransition: http.StatusConflict,
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		status, ok := statusByCode[de.Code]
		if !ok {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, errorView{errorDetail{Code: string(de.Code), Message: de.Message, RequestID: requestID(r.Context())}})
		return
	}
	// The full error goes to the operator's log; the user gets the request ID to look it up.
	info := requestInfoFrom(r.Context())
	id := info.id
	s.log.ErrorContext(r.Context(), "request failed",
		append(info.logAttrs(), "method", r.Method, "path", r.URL.Path, "err", service.RedactSecrets(err.Error()))...)
	writeJSON(w, http.StatusInternalServerError, errorView{errorDetail{
		Code: "internal", Message: "internal error; give the operator this request ID to find the full log", RequestID: id,
	}})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.Errorf(domain.CodeInvalid, "invalid JSON body: %v", err)
	}
	if dec.More() {
		return domain.Errorf(domain.CodeInvalid, "body must be a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
