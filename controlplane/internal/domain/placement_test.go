package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestRegionIDs(t *testing.T) {
	for r, ok := range map[Region]bool{"local": true, "in-mum-1": true, "eu-fra-1": true, "": false, "EU": false, "us east": false, "-x": false} {
		if r.Valid() != ok {
			t.Errorf("Region(%q).Valid() = %v, want %v", r, !ok, ok)
		}
	}
}

func TestMachineValidate(t *testing.T) {
	good := Machine{
		ID: uuid.New(), TenantID: uuid.New(), Name: "studio-mac", Region: RegionLocal, OS: "darwin", Arch: "arm64",
		Accelerators: []Accelerator{{Kind: AcceleratorApple, Model: "Apple M3 Max", Count: 1, MemoryMiB: 131072}},
		Sharing:      SharingPrivate,
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid machine: %v", err)
	}
	tests := map[string]func(*Machine){
		"no region":         func(m *Machine) { m.Region = "" }, // a machine belongs to exactly one region
		"bad region":        func(m *Machine) { m.Region = "Mumbai 1" },
		"windows":           func(m *Machine) { m.OS = "windows" },
		"32-bit":            func(m *Machine) { m.Arch = "386" },
		"unknown sharing":   func(m *Machine) { m.Sharing = "public" },
		"unknown device":    func(m *Machine) { m.Accelerators = []Accelerator{{Kind: "tpu", Count: 1}} },
		"zero devices":      func(m *Machine) { m.Accelerators = []Accelerator{{Kind: AcceleratorNVIDIA, Count: 0}} },
		"name with a space": func(m *Machine) { m.Name = "my mac" },
	}
	for name, mutate := range tests {
		m := good
		mutate(&m)
		if code, _ := CodeOf(m.Validate()); code != CodeInvalid {
			t.Errorf("%s: Validate() code = %q, want %q", name, code, CodeInvalid)
		}
	}
}

// Usage is accounted for what actually ran, so nothing is metered before it's placed (ADR-0010).
func TestMeteredWorkloadsMustBePlaced(t *testing.T) {
	w := Workload{ID: uuid.New(), GPUClass: "l40s-48gb"}
	for _, s := range AllStates() {
		err := w.CheckPlaced(s)
		if s.Metered() && err == nil {
			t.Errorf("%s: an unplaced workload must not enter a metered state", s)
		}
		if !s.Metered() && err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	w.PlacedClass = "l40s-48gb"
	if err := w.CheckPlaced(StateRunning); err != nil {
		t.Errorf("placed workload: %v", err)
	}
}
