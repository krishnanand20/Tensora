package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tensora/tensora/controlplane/internal/api"
	"github.com/tensora/tensora/controlplane/internal/auth"
	"github.com/tensora/tensora/controlplane/internal/service"
	"github.com/tensora/tensora/controlplane/internal/testutil"
)

func newHandler(t *testing.T) http.Handler {
	st := testutil.SeededStore(t)
	svc := service.New(st, testutil.Catalog(t), nil)
	return api.New(svc, auth.Dev{}, slog.New(slog.DiscardHandler), st.Ping).Handler()
}

func call(t *testing.T, h http.Handler, method, path, tenant, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tenant != "" {
		req.Header.Set("X-Dev-Tenant", tenant)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: non-JSON response %q", method, path, rec.Body.String())
	}
	return rec.Code, out
}

func errCode(body map[string]any) any {
	e, _ := body["error"].(map[string]any)
	return e["code"]
}

const endpointBody = `{"name":"llm","kind":"endpoint","image_id":"vllm","gpu_class":"l40s-48gb","gpu_count":1,"model_uri":"hf://Qwen/Qwen2.5-0.5B-Instruct"}`

func TestHealth(t *testing.T) {
	h := newHandler(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		if code, _ := call(t, h, "GET", path, "", ""); code != http.StatusOK {
			t.Errorf("%s = %d", path, code)
		}
	}
}

func TestAuthErrors(t *testing.T) {
	h := newHandler(t)
	if code, body := call(t, h, "GET", "/v1/workloads", "", ""); code != 401 || errCode(body) != "unauthenticated" {
		t.Errorf("no auth: %d %v", code, body)
	}
	if code, body := call(t, h, "GET", "/v1/workloads", "nobody", ""); code != 403 || errCode(body) != "forbidden" {
		t.Errorf("unknown tenant: %d %v", code, body)
	}
}

func TestWorkloadCRUDAndTenantIsolation(t *testing.T) {
	h := newHandler(t)
	code, created := call(t, h, "POST", "/v1/workloads", "acme", endpointBody)
	if code != http.StatusCreated || created["state"] != "pending" {
		t.Fatalf("create: %d %v", code, created)
	}
	for _, leaked := range []string{"params", "tenant_id", "created_by"} {
		if _, ok := created[leaked]; ok {
			t.Errorf("response leaks %q", leaked)
		}
	}
	id := created["id"].(string)

	if code, list := call(t, h, "GET", "/v1/workloads", "acme", ""); code != 200 || len(list["items"].([]any)) != 1 {
		t.Errorf("acme list: %d %v", code, list)
	}
	if code, list := call(t, h, "GET", "/v1/workloads", "globex", ""); code != 200 || len(list["items"].([]any)) != 0 {
		t.Errorf("globex list must be empty: %d %v", code, list)
	}
	if code, body := call(t, h, "GET", "/v1/workloads/"+id, "globex", ""); code != 404 {
		t.Errorf("cross-tenant get must be 404, got %d %v", code, body)
	}
	if code, body := call(t, h, "POST", "/v1/workloads/"+id+"/stop", "acme", ""); code != 409 || errCode(body) != "invalid_transition" {
		t.Errorf("stop pending: %d %v", code, body)
	}
	if code, body := call(t, h, "DELETE", "/v1/workloads/"+id, "acme", ""); code != 202 || body["state"] != "deleting" {
		t.Errorf("delete: %d %v", code, body)
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(endpointBody, `"name"`, `"trust_remote_code":true,"name"`, 1)
	if code, resp := call(t, h, "POST", "/v1/workloads", "acme", body); code != 400 || errCode(resp) != "invalid_request" {
		t.Errorf("unknown field: %d %v", code, resp)
	}
}

func TestCatalogHidesInternalsAndInternalOnlyClasses(t *testing.T) {
	h := newHandler(t)
	ids := func(tenant string) map[string]bool {
		_, body := call(t, h, "GET", "/v1/catalog", tenant, "")
		out := map[string]bool{}
		for _, g := range body["gpu_classes"].([]any) {
			class := g.(map[string]any)
			for _, internal := range []string{"vm_device_name", "node_selector", "container_resource"} {
				if _, ok := class[internal]; ok {
					t.Errorf("catalog leaks %s", internal)
				}
			}
			out[class["id"].(string)] = true
		}
		return out
	}
	if ids("acme")["rtx4090-24gb"] {
		t.Error("untrusted tenant sees internal-only GPU class")
	}
	if !ids("internal")["rtx4090-24gb"] {
		t.Error("trusted tenant should see internal-only GPU class")
	}
}
