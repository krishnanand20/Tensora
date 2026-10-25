package redact

import (
	"strings"
	"testing"
)

func TestSecrets(t *testing.T) {
	got := Secrets(`pull failed for s3://AKIA:hunter2@models/m?X-Amz-Signature=abc&region=x: denied`)
	if strings.Contains(got, "hunter2") || strings.Contains(got, "abc") || !strings.Contains(got, "region=x") {
		t.Errorf("Secrets = %q", got)
	}
	if clean, reasons := URI("hf://Qwen/Qwen2.5-0.5B-Instruct"); clean != "hf://Qwen/Qwen2.5-0.5B-Instruct" || len(reasons) != 0 {
		t.Errorf("a URI without credentials changed: %q %v", clean, reasons)
	}
}
