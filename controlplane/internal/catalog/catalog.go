// Package catalog loads images/catalog.yaml: the GPU classes and images users can launch.
package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/tensora/tensora/controlplane/internal/domain"
)

const defaultRootDiskGiB = 100

type GPUClass struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	VRAMGB      int    `json:"vram_gb"`
	// ContainerResource is the extended resource requested by notebook/endpoint pods.
	ContainerResource string `json:"container_resource"`
	// VMDeviceName is the KubeVirt host-device resource for passthrough. Empty = no VMs.
	VMDeviceName string `json:"vm_device_name,omitempty"`
	// NodeSelector picks container-mode nodes carrying this GPU (GPU Feature Discovery labels).
	NodeSelector    map[string]string `json:"node_selector,omitempty"`
	VCPUsPerGPU     int               `json:"vcpus_per_gpu"`
	MemoryGiBPerGPU int               `json:"memory_gib_per_gpu"`
	// InternalOnly hides the class from untrusted tenants (e.g. consumer GPUs, see gpu-sharing.md).
	InternalOnly bool `json:"internal_only,omitempty"`
}

// VisibleTo reports whether tenants in tier may see and use this class.
func (g GPUClass) VisibleTo(tier domain.IsolationTier) bool {
	return !g.InternalOnly || tier == domain.TierTrusted
}

type Image struct {
	ID          string              `json:"id"`
	DisplayName string              `json:"display_name"`
	Kind        domain.WorkloadKind `json:"kind"`
	// Ref depends on Kind: vm → CDI DataSource name, notebook → container image,
	// endpoint → KServe ClusterServingRuntime name.
	Ref         string `json:"ref"`
	Description string `json:"description,omitempty"`
	RootDiskGiB int    `json:"root_disk_gib,omitempty"`
}

type Catalog struct {
	GPUClasses []GPUClass `json:"gpu_classes"`
	Images     []Image    `json:"images"`
}

// Load reads and validates a catalog file. Unknown fields are errors.
func Load(path string) (*Catalog, error) {
	// The path comes from operator config (TENSORA_CATALOG_PATH), never from a request.
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var c Catalog
	if err := yaml.UnmarshalStrict(raw, &c); err != nil {
		return nil, fmt.Errorf("parse catalog %s: %w", path, err)
	}
	for i := range c.Images {
		if c.Images[i].Kind == domain.KindVM && c.Images[i].RootDiskGiB == 0 {
			c.Images[i].RootDiskGiB = defaultRootDiskGiB
		}
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid catalog %s: %w", path, err)
	}
	return &c, nil
}

func (c *Catalog) Validate() error {
	var errs []error
	seen := map[string]bool{}
	for _, g := range c.GPUClasses {
		switch {
		case g.ID == "" || seen[g.ID]:
			errs = append(errs, fmt.Errorf("gpu class %q: missing or duplicate id", g.ID))
		case g.ContainerResource == "":
			errs = append(errs, fmt.Errorf("gpu class %q: container_resource is required", g.ID))
		case g.VCPUsPerGPU <= 0 || g.MemoryGiBPerGPU <= 0:
			errs = append(errs, fmt.Errorf("gpu class %q: vcpus_per_gpu and memory_gib_per_gpu must be positive", g.ID))
		}
		seen[g.ID] = true
	}
	seen = map[string]bool{}
	for _, img := range c.Images {
		switch {
		case img.ID == "" || seen[img.ID]:
			errs = append(errs, fmt.Errorf("image %q: missing or duplicate id", img.ID))
		case !img.Kind.Valid():
			errs = append(errs, fmt.Errorf("image %q: invalid kind %q", img.ID, img.Kind))
		case img.Ref == "":
			errs = append(errs, fmt.Errorf("image %q: ref is required", img.ID))
		}
		seen[img.ID] = true
	}
	return errors.Join(errs...)
}

func (c *Catalog) GPUClass(id string) (GPUClass, bool) {
	for _, g := range c.GPUClasses {
		if g.ID == id {
			return g, true
		}
	}
	return GPUClass{}, false
}

func (c *Catalog) Image(id string) (Image, bool) {
	for _, img := range c.Images {
		if img.ID == id {
			return img, true
		}
	}
	return Image{}, false
}
