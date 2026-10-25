package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/tensora/tensora/controlplane/internal/catalog"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

// The real catalog: an endpoint is served by vLLM on NVIDIA, MLX on Apple Silicon, and llama.cpp
// on CPU (ADR-0013).
func TestEndpointRuntimePerAccelerator(t *testing.T) {
	cat := testutil.Catalog(t)
	img, ok := cat.Image("openai-endpoint")
	if !ok {
		t.Fatal("openai-endpoint missing from the catalog")
	}
	for accel, want := range map[domain.AcceleratorKind]string{
		domain.AcceleratorNVIDIA: "vllm", domain.AcceleratorApple: "mlx", domain.AcceleratorCPU: "llama.cpp", domain.AcceleratorAMD: "llama.cpp",
	} {
		if r, ok := img.RuntimeFor(accel); !ok || r.Server != want {
			t.Errorf("%s: runtime = %+v, want %s", accel, r, want)
		}
	}
	if cat.DefaultRegion() != domain.RegionLocal {
		t.Errorf("default region = %q, want local", cat.DefaultRegion())
	}
}

func TestValidate(t *testing.T) {
	valid := func() catalog.Catalog {
		return catalog.Catalog{
			Regions: []catalog.Region{{ID: "local"}, {ID: "eu-fra-1"}},
			GPUClasses: []catalog.GPUClass{
				{ID: "l40s", Accelerator: domain.AcceleratorNVIDIA, VRAMGB: 48, ContainerResource: "nvidia.com/gpu", VCPUsPerGPU: 8, MemoryGiBPerGPU: 64, Regions: []domain.Region{"eu-fra-1"}},
				{ID: "mac", Accelerator: domain.AcceleratorApple, VRAMGB: 32, VCPUsPerGPU: 8, MemoryGiBPerGPU: 32},
				{ID: "cpu", Accelerator: domain.AcceleratorCPU, VCPUsPerGPU: 4, MemoryGiBPerGPU: 16},
			},
			Images: []catalog.Image{{ID: "ep", Kind: domain.KindEndpoint, Runtimes: []catalog.Runtime{
				{Accelerators: []domain.AcceleratorKind{domain.AcceleratorNVIDIA}, Server: "vllm", ModelFormat: "safetensors", Ref: "tensora-vllm"},
			}}},
		}
	}
	c := valid()
	if err := c.Validate(); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
	tests := map[string]struct {
		mutate func(*catalog.Catalog)
		want   string
	}{
		"no regions":              {func(c *catalog.Catalog) { c.Regions = nil }, "at least one"},
		"bad region id":           {func(c *catalog.Catalog) { c.Regions[1].ID = "EU Frankfurt" }, "invalid or duplicate"},
		"class in unknown region": {func(c *catalog.Catalog) { c.GPUClasses[0].Regions = []domain.Region{"us-east-1"} }, "isn't in the catalog"},
		"no accelerator":          {func(c *catalog.Catalog) { c.GPUClasses[0].Accelerator = "" }, "accelerator must be"},
		"nvidia without resource": {func(c *catalog.Catalog) { c.GPUClasses[0].ContainerResource = "" }, "container_resource is required"},
		"apple on kubernetes":     {func(c *catalog.Catalog) { c.GPUClasses[1].ContainerResource = "nvidia.com/gpu" }, "only on node agents"},
		"gpu without memory":      {func(c *catalog.Catalog) { c.GPUClasses[1].VRAMGB = 0 }, "vram_gb must be positive"},
		"image without runtimes":  {func(c *catalog.Catalog) { c.Images[0].Runtimes = nil }, "at least one runtime"},
		"endpoint without server": {func(c *catalog.Catalog) { c.Images[0].Runtimes[0].Server = "" }, "name their model server"},
		"unknown accelerator":     {func(c *catalog.Catalog) { c.Images[0].Runtimes[0].Accelerators = []domain.AcceleratorKind{"tpu"} }, "unknown accelerator"},
		"endpoint without format": {func(c *catalog.Catalog) { c.Images[0].Runtimes[0].ModelFormat = "" }, "model_format must be"},
	}
	for name, tt := range tests {
		c := valid()
		tt.mutate(&c)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want mention of %q", name, err, tt.want)
		}
	}
}

// repoFile finds path relative to the repository root, above the test's working directory.
func repoFile(t *testing.T, path string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			return filepath.Join(dir, path)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("%s not found above the working directory", path)
		}
		dir = parent
	}
}

// The Datacenter catalog offers only what its Kubernetes backend can run, and lists every
// environment's region, so no workload is created somewhere no controller serves.
func TestDatacenterCatalog(t *testing.T) {
	cat, err := catalog.Load(repoFile(t, "deploy/catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range cat.GPUClasses {
		if g.ContainerResource == "" {
			t.Errorf("class %s has no Kubernetes resource, so the Datacenter can't run it", g.ID)
		}
	}
	envs, err := filepath.Glob(filepath.Join(filepath.Dir(repoFile(t, "deploy/catalog.yaml")), "environments", "*.yaml"))
	if err != nil || len(envs) == 0 {
		t.Fatalf("environment files: %v, %v", envs, err)
	}
	for _, env := range envs {
		raw, err := os.ReadFile(env) //nolint:gosec // a path the test globbed from the repository
		if err != nil {
			t.Fatal(err)
		}
		var values struct {
			Region domain.Region `json:"region"`
		}
		if err := yaml.Unmarshal(raw, &values); err != nil {
			t.Fatalf("%s: %v", env, err)
		}
		if _, ok := cat.Region(values.Region); !ok {
			t.Errorf("%s: region %q isn't in deploy/catalog.yaml", filepath.Base(env), values.Region)
		}
	}
}

// The launch rules (rule 4, rule 6, ADR-0009), against the real catalog.
func TestCheckLaunch(t *testing.T) {
	cat := testutil.Catalog(t)
	class := func(id string) catalog.GPUClass {
		g, ok := cat.GPUClass(id)
		if !ok {
			t.Fatalf("class %s missing", id)
		}
		return g
	}
	image := func(id string) catalog.Image {
		img, ok := cat.Image(id)
		if !ok {
			t.Fatalf("image %s missing", id)
		}
		return img
	}
	macVM := catalog.Image{ID: "macos", Kind: domain.KindVM, Runtimes: []catalog.Runtime{
		{Accelerators: []domain.AcceleratorKind{domain.AcceleratorApple}, Ref: "macos-15"},
	}}
	tests := []struct {
		name   string
		tier   domain.IsolationTier
		class  catalog.GPUClass
		image  catalog.Image
		region domain.Region
		want   string // "" = allowed
		server string
	}{
		{"owner's notebook on a Mac", domain.TierTrusted, class("apple-silicon-32gb"), image("pytorch-notebook"), "local", "", ""},
		{"owner's endpoint on a Mac uses MLX", domain.TierTrusted, class("apple-silicon-32gb"), image("openai-endpoint"), "local", "", "mlx"},
		{"invited user's notebook on a Mac", domain.TierUntrusted, func() catalog.GPUClass {
			g := class("apple-silicon-32gb")
			g.InternalOnly = false // even if an operator offers Apple classes to everyone
			return g
		}(), image("pytorch-notebook"), "local", "only the machine's owner", ""},
		{"invited user's endpoint on a Mac", domain.TierUntrusted, func() catalog.GPUClass {
			g := class("apple-silicon-32gb")
			g.InternalOnly = false
			return g
		}(), image("openai-endpoint"), "local", "only the machine's owner", ""},
		{"invited user's macOS VM", domain.TierUntrusted, func() catalog.GPUClass {
			g := class("apple-silicon-32gb")
			g.InternalOnly = false
			return g
		}(), macVM, "local", "", ""},
		{"hidden class", domain.TierUntrusted, class("apple-silicon-32gb"), image("openai-endpoint"), "local", "isn't available", ""},
		{"CPU endpoint needs GGUF", domain.TierTrusted, class("cpu-4c-16g"), image("openai-endpoint"), "local", "aren't accepted until", ""},
		{"NVIDIA endpoint uses vLLM", domain.TierUntrusted, class("l40s-48gb"), image("openai-endpoint"), "local", "", "vllm"},
		{"unknown region", domain.TierTrusted, func() catalog.GPUClass {
			g := class("l40s-48gb")
			g.Regions = []domain.Region{"local"}
			return g
		}(), image("openai-endpoint"), "eu-fra-1", "isn't offered in region", ""},
	}
	for _, tt := range tests {
		runtime, err := catalog.CheckLaunch(tt.tier, tt.class, tt.image, tt.image.Kind, tt.region)
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		case runtime.Server != tt.server:
			t.Errorf("%s: server = %q, want %q", tt.name, runtime.Server, tt.server)
		}
	}
}

// The workload's own kind is what's checked: an image whose kind changed in the catalog can't turn
// an untrusted notebook on a Mac into an allowed "VM".
func TestCheckLaunchUsesTheWorkloadsKind(t *testing.T) {
	g, _ := testutil.Catalog(t).GPUClass("apple-silicon-32gb")
	g.InternalOnly = false
	nowAVM := catalog.Image{ID: "pytorch-notebook", Kind: domain.KindVM, Runtimes: []catalog.Runtime{
		{Accelerators: []domain.AcceleratorKind{domain.AcceleratorApple}, Ref: "x"},
	}}
	if _, err := catalog.CheckLaunch(domain.TierUntrusted, g, nowAVM, domain.KindNotebook, "local"); err == nil ||
		!strings.Contains(err.Error(), "no longer a notebook image") {
		t.Fatalf("err = %v", err)
	}
}
