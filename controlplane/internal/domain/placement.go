package domain

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Region is one location of a deployment: a home, an office, or a datacenter (ADR-0009). Every
// workload and machine belongs to exactly one, and a workload's data never leaves it (rule 9).
type Region string

// RegionLocal is the only region of a Solo install.
const RegionLocal Region = "local"

// Valid reports whether r is a region ID: lowercase and location-based, like local or in-mum-1.
func (r Region) Valid() bool { return ValidSlug(string(r)) }

// AcceleratorKind is the kind of compute a machine offers or a class needs (ADR-0013).
type AcceleratorKind string

const (
	AcceleratorNVIDIA AcceleratorKind = "nvidia"
	AcceleratorApple  AcceleratorKind = "apple" // the Apple Silicon GPU (Metal), with unified memory
	AcceleratorAMD    AcceleratorKind = "amd"
	AcceleratorCPU    AcceleratorKind = "cpu"
)

func (k AcceleratorKind) Valid() bool {
	switch k {
	case AcceleratorNVIDIA, AcceleratorApple, AcceleratorAMD, AcceleratorCPU:
		return true
	}
	return false
}

// Accelerator is one kind of device in a machine's inventory.
type Accelerator struct {
	Kind      AcceleratorKind `json:"kind"`
	Model     string          `json:"model"` // as the driver reports it, for example "NVIDIA L40S" or "Apple M3 Max"
	Count     int             `json:"count"`
	MemoryMiB int64           `json:"memory_mib"` // per device; unified memory for Apple
}

// SharingLevel decides who may run work on a machine (ADR-0014).
type SharingLevel string

const (
	SharingPrivate   SharingLevel = "private"   // the owner's tenant only (the default)
	SharingInvite    SharingLevel = "invite"    // named users, groups, or tenants
	SharingCommunity SharingLevel = "community" // anyone with an account on the instance
)

func (l SharingLevel) Valid() bool {
	switch l {
	case SharingPrivate, SharingInvite, SharingCommunity:
		return true
	}
	return false
}

// Machine is a computer that runs workloads through the node agent (ADR-0013). Its owner is one
// tenant, and it belongs to exactly one region.
type Machine struct {
	ID           uuid.UUID
	TenantID     uuid.UUID // the owner
	Name         string
	Region       Region
	OS           string // "linux" or "darwin"
	Arch         string // "amd64" or "arm64"
	Accelerators []Accelerator
	Sharing      SharingLevel
	LastSeenAt   time.Time // zero until the agent's first heartbeat
	CreatedAt    time.Time
}

// Validate checks a machine before it's stored. The error is safe to show its owner.
func (m Machine) Validate() error {
	switch {
	case !ValidSlug(m.Name):
		return Errorf(CodeInvalid, "machine name must be 1-40 lowercase letters, digits, or '-'")
	case !m.Region.Valid():
		return Errorf(CodeInvalid, "machine %s needs exactly one valid region, got %q", m.Name, m.Region)
	case m.OS != "linux" && m.OS != "darwin":
		return Errorf(CodeInvalid, "machine %s: os must be linux or darwin, got %q", m.Name, m.OS)
	case m.Arch != "amd64" && m.Arch != "arm64":
		return Errorf(CodeInvalid, "machine %s: arch must be amd64 or arm64, got %q", m.Name, m.Arch)
	case !m.Sharing.Valid():
		return Errorf(CodeInvalid, "machine %s: sharing must be private, invite, or community, got %q", m.Name, m.Sharing)
	}
	for i, a := range m.Accelerators {
		// The agent reports these, and a host may be hostile: bound the text, and never echo it.
		if !a.Kind.Valid() || a.Count < 1 || a.MemoryMiB < 0 || len(a.Model) > maxModelLen ||
			strings.IndexFunc(a.Model, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
			return Errorf(CodeInvalid, "machine %s: accelerator %d is invalid", m.Name, i)
		}
	}
	return nil
}

const maxModelLen = 100

// CanHost reports whether the machine may run w: it must be in w's region (rule 9), and a machine
// that isn't in a community pool runs only its owner's workloads (ADR-0014). Invite lists arrive
// with sharing (Phase 4); until then an invite-only machine is treated as private.
func (m Machine) CanHost(w Workload) error {
	switch {
	case m.Region != w.Region:
		return Errorf(CodeInvalid, "machine %s is in region %s, not the workload's region %s", m.Name, m.Region, w.Region)
	case m.TenantID != w.TenantID && m.Sharing != SharingCommunity:
		return Errorf(CodeForbidden, "machine %s isn't shared with this workload's tenant", m.Name)
	}
	return nil
}

// CheckPlaced reports whether a workload may enter state to. Once it's provisioning it holds
// capacity, and once it's running it's metered, so from then on it must say what it runs on:
// capacity is counted and usage accounted for what actually runs (ADR-0010).
func (w Workload) CheckPlaced(to State) error {
	if (to == StateProvisioning || to.Metered()) && w.PlacedClass == "" {
		return Errorf(CodeInvalidTransition, "workload %s can't be %s before it's placed on a class", w.ID, to)
	}
	return nil
}
