# AGENTS.md: ml/ (Python ML layer)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

## What lives here

ML-specific tooling that the Go core platform orchestrates but does not implement:

- `validate.py` is **security-critical**. It decides which model files a tenant may serve. It's an allowlist: new file types are rejected unless explicitly added. Pickle formats and `.py` files are never allowed, and neither is `auto_map` or `trust_remote_code` in configs.
- `ingest.py`: download → validate → upload with a checksum manifest. It uploads only the files the validator accepted, and only to the object store **in the region where the model will be served** (ADR-0009).
- The validator's `total_bytes` will also decide whether a model is small enough for **CPU fallback** (ADR-0010). Keep it accurate.
- `bench.py`: endpoint load testing (TTFT, latency, throughput).
- `burnin.py`: GPU acceptance tests for new nodes. Needs the `burnin` extra (torch).

## Rules

- **Changes to `validate.py`** need a test that proves the new behavior, plus review from someone who knows the isolation model (see CONTRIBUTING.md). Never loosen the allowlist to make one model work. Ask the customer to convert the model instead.
- **Heavy dependencies** (`torch`, `transformers`) are optional extras and imported inside functions, so `validate`, `ingest`, and `bench` stay light and testable without a GPU.
- **Never load model weights** in the validator. Inspect headers and metadata only. Loading is exactly what an attacker wants us to do.
- **Credentials** come from the environment (`AWS_*`, `HF_TOKEN`, `TENSORA_BENCH_API_KEY`). Never pass them as CLI arguments, because those show up in process lists and Job specs.
- **Output** is JSON on stdout, logs go to stderr, and a failed check exits with code 1. The controller will parse this when it runs these commands as Jobs.
- Python 3.12, full type hints, `mypy --strict` must pass, and code is formatted with ruff.

## Commands

```bash
uv sync
uv run pytest
uv run ruff check . && uv run ruff format --check . && uv run mypy
```
