# ml: Tensora ML layer (Python)

The ML-specific work behind the platform, shipped as the `tensora-ml` CLI and container image:

| Command | What it does | Where it runs |
|---|---|---|
| `tensora-ml validate <dir>` | Enforces the model policy: safetensors only, no pickle files, no custom code, sane headers, size limit | anywhere |
| `tensora-ml ingest hf://org/model[@rev] s3://bucket/prefix` | Downloads only allowed file types, validates them, then uploads with a sha256 manifest | operator laptop now; Kubernetes Job from Phase 3 |
| `tensora-ml bench <url> --model <name>` | Load-tests an OpenAI-compatible endpoint and reports TTFT p50/p95 and tokens/s | anywhere with network access |
| `tensora-ml burnin` | Runs sustained bf16 matmul, NaN checks, and a VRAM pattern test on every visible GPU | new GPU nodes in container mode |

All output is JSON. A failed check exits with code 1.

## Develop

```bash
uv sync                  # add --extra burnin on a GPU machine
uv run pytest
uv run ruff check . && uv run mypy
uv run tensora-ml validate ./some-model
```

## Ingest a model into the in-cluster object store

```bash
export AWS_ENDPOINT_URL=https://s3.tensora.internal AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...
uv run tensora-ml ingest hf://Qwen/Qwen2.5-0.5B-Instruct@<commit-sha> s3://tensora-models/internal/qwen2.5-0.5b
```

Pin a commit SHA. Branch names like `main` can change under you.
