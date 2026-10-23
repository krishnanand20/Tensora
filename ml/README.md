# ml: Tensora ML layer (Python)

The ML-specific work behind the platform, shipped as the `tensora-ml` CLI. It runs on macOS
(Apple Silicon) and Linux, and is used by agents and by datacenter jobs.

| Command | What it does | Where it runs |
|---|---|---|
| `tensora-ml validate <dir>` | Enforces the model policy: code-free formats only (safetensors today; GGUF planned), no pickle, no custom code, sane headers, a size limit | Anywhere |
| `tensora-ml ingest hf://org/model[@rev] s3://bucket/prefix` | Downloads only allowed file types, validates them, then stores them with a sha256 manifest | A laptop today; agents and datacenter jobs later |
| `tensora-ml bench <url> --model <name>` | Load-tests any OpenAI-compatible endpoint (vLLM, llama.cpp, MLX) and reports TTFT p50/p95 and tokens/s | Anywhere with network access |
| `tensora-ml burnin` | Sustained bf16 matmul, NaN checks, and a memory pattern test on every visible GPU | NVIDIA GPUs today; Apple GPUs (MPS) planned |

All output is JSON. A failed check exits with code 1.

## Develop

```bash
uv sync                  # add --extra burnin on a GPU machine
uv run pytest
uv run ruff check . && uv run mypy
uv run tensora-ml validate ./some-model
uv run tensora-ml bench http://127.0.0.1:8080 --model my-model   # e.g. a local llama.cpp server
```

## Ingest a model into a datacenter object store

```bash
export AWS_ENDPOINT_URL=https://s3.tensora.internal AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...
uv run tensora-ml ingest hf://Qwen/Qwen2.5-0.5B-Instruct@<commit-sha> s3://tensora-models/internal/qwen2.5-0.5b
```

Pin a commit SHA, because branch names like `main` can change under you. Store models in the region where
they'll be served, because model data never leaves its region (ADR-0009).
