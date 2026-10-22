package config

import (
	"strings"
	"testing"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestDefaultsAreAValidDevConfig(t *testing.T) {
	c, err := FromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "dev" || c.AuthMode != "dev" || c.ClusterBackend != "fake" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.FakeCapacity["l40s-48gb"] != 4 {
		t.Fatalf("fake capacity = %v", c.FakeCapacity)
	}
}

func TestProductionGuards(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"dev auth outside dev", map[string]string{"TENSORA_ENV": "production", "TENSORA_DATABASE_URL": "postgres://x"}, "AUTH_MODE=dev"},
		{"fake cluster in prod", map[string]string{
			"TENSORA_ENV": "production", "TENSORA_DATABASE_URL": "postgres://x",
			"TENSORA_AUTH_MODE": "oidc", "TENSORA_OIDC_ISSUER": "https://auth",
		}, "CLUSTER_BACKEND=fake"},
		{"memory store outside dev", map[string]string{
			"TENSORA_ENV": "staging", "TENSORA_AUTH_MODE": "oidc", "TENSORA_OIDC_ISSUER": "https://auth",
		}, "DATABASE_URL is required"},
		{"oidc without issuer", map[string]string{"TENSORA_AUTH_MODE": "oidc"}, "OIDC_ISSUER"},
		{"bad capacity", map[string]string{"TENSORA_FAKE_CAPACITY": "l40s"}, "FAKE_CAPACITY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromEnv(env(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want mention of %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidProductionConfig(t *testing.T) {
	_, err := FromEnv(env(map[string]string{
		"TENSORA_ENV": "production", "TENSORA_DATABASE_URL": "postgres://x",
		"TENSORA_AUTH_MODE": "oidc", "TENSORA_OIDC_ISSUER": "https://auth/realms/tensora",
		"TENSORA_CLUSTER_BACKEND": "kubernetes",
	}))
	if err != nil {
		t.Fatal(err)
	}
}
