// Package domain holds Tensora's core types and rules. It depends on nothing else in the
// module, so storage, HTTP, Kubernetes, and metering code can all depend on it.
package domain

import (
	"regexp"
	"time"

	"github.com/google/uuid"
)

// WorkloadKind is what a customer launches.
type WorkloadKind string

const (
	KindVM       WorkloadKind = "vm"       // KubeVirt VM with whole-GPU passthrough
	KindNotebook WorkloadKind = "notebook" // JupyterLab in a (Kata) container
	KindEndpoint WorkloadKind = "endpoint" // KServe InferenceService running vLLM
)

func (k WorkloadKind) Valid() bool {
	switch k {
	case KindVM, KindNotebook, KindEndpoint:
		return true
	}
	return false
}

// IsolationTier decides which runtimes a tenant's code may use.
type IsolationTier string

const (
	// TierTrusted is our own team: plain runc containers are allowed.
	TierTrusted IsolationTier = "trusted"
	// TierUntrusted is every external customer: KubeVirt VMs or Kata containers only.
	TierUntrusted IsolationTier = "untrusted"
)

func (t IsolationTier) Valid() bool { return t == TierTrusted || t == TierUntrusted }

// slugPattern keeps tenant slugs and workload names safe to use as DNS labels.
var slugPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$`)

// ValidSlug reports whether s is 1-40 lowercase alphanumerics or '-', not starting or ending with '-'.
func ValidSlug(s string) bool { return slugPattern.MatchString(s) }

type Tenant struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	Tier      IsolationTier
	CreatedAt time.Time
}

// Quota caps how many GPUs of one class a tenant may hold at once.
type Quota struct {
	TenantID uuid.UUID
	GPUClass string
	MaxGPUs  int
}

type Workload struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string
	Kind      WorkloadKind
	State     State
	ImageID   string
	GPUClass  string
	GPUCount  int
	Params    map[string]string // kind-specific inputs, keys below
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time // also when the current state was entered
}

// Keys used in Workload.Params.
const (
	ParamSSHPublicKey = "ssh_public_key" // vm
	ParamModelURI     = "model_uri"      // endpoint
)

// Event is one append-only row in workload_events. Metering is computed from these.
type Event struct {
	ID         int64
	WorkloadID uuid.UUID
	From       State // empty for the creation event
	To         State
	Reason     string
	At         time.Time
}

// UsageRecord is the billable time of one workload within one UTC clock hour.
type UsageRecord struct {
	WorkloadID  uuid.UUID
	PeriodStart time.Time
	TenantID    uuid.UUID
	GPUClass    string
	GPUCount    int
	Seconds     int64
}

// GPUSeconds is the quantity that gets priced.
func (u UsageRecord) GPUSeconds() int64 { return u.Seconds * int64(u.GPUCount) }

// Principal is an authenticated caller.
type Principal struct {
	Subject    string
	Email      string
	TenantSlug string
}
