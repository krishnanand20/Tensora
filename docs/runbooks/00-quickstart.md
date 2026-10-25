# 00: Quickstart (Solo: one Mac or Linux machine)

> **Status: planned (Phase 1).** The install commands below describe the target experience and don't
> work yet. What works **today** on macOS and Linux is the development stack (see the end of this page).
> Turn this runbook into a verified one when Phase 1 lands.

## Requirements

| Machine | Minimum | For bigger models |
|---|---|---|
| Mac | Apple Silicon (M1 or newer), macOS 14+, 16 GB unified memory, 30 GB free disk | 64 GB+ unified memory |
| Linux + NVIDIA | Ubuntu 22.04+ (or similar), NVIDIA driver installed, Docker with the NVIDIA container toolkit, 16 GB RAM | A GPU with 24 GB+ VRAM |
| Linux, CPU only | Any 64-bit Linux with Docker, 16 GB RAM | — |

## Install (target experience)

```bash
# macOS
brew install tensora/tap/tensora

# Linux
curl -fsSL https://get.tensora.example/install.sh | sh
```

## Start

```bash
tensora up
```

`tensora up` does the following:

- creates its data directory (macOS: `~/Library/Application Support/Tensora`; Linux: `~/.local/share/tensora`)
- starts the control plane and the local agent in one process, on `127.0.0.1` only
- asks you to create the first account
- opens the dashboard

## Launch something

1. **Notebook:** Launch → Notebook → "PyTorch + JupyterLab". It uses Metal on a Mac, CUDA on NVIDIA, or CPU.
2. **Model endpoint:** Launch → Model endpoint. Pick a small model (for example a 0.5B–3B instruct model in GGUF or MLX format). You get an OpenAI-compatible URL:

   ```bash
   curl -s http://127.0.0.1:<port>/v1/chat/completions -H 'Content-Type: application/json' \
     -d '{"model":"<name>","messages":[{"role":"user","content":"Hello"}]}'
   ```

3. **VM** (Phase 2): Launch → GPU VM.

## See what happened

Every workload keeps a full timeline: each state change, when, why, and by whom. Open the workload in
the dashboard and click **Download full log** to save everything about it, or use the CLI:

```bash
tensora events <id>      # the timeline
tensora export <id>      # a complete copy, as JSON
tensora workloads -all   # deleted workloads too: their history stays available
```

If something fails, the error shows a request ID. It points to the exact lines in the log
(`TENSORA_LOG_LEVEL=debug` for more detail). See [logs and transparency](../architecture/logs-and-transparency.md).

## Keep it private

`tensora up` listens only on localhost, and your machine is **private** until you choose otherwise.
To add more machines, or to share with others, see the Lab and sharing sections of the
[product spec](../product/product-spec.md#how-you-deploy-it).

## What works today

The development stack runs on macOS (Apple Silicon) and Linux. It uses a fake backend, so nothing
actually runs on your GPU yet:

```bash
brew install go uv node       # on a Mac, if `make doctor` reports them missing
make setup && make dev        # API on :8080
make web-dev                  # dashboard on :5173
```

The CLI's read commands work today against `make dev`:

```bash
cd controlplane && go build -o bin/tensora ./cmd/tensora
TENSORA_DEV_TENANT=acme bin/tensora workloads
TENSORA_DEV_TENANT=acme bin/tensora events <id>
TENSORA_DEV_TENANT=acme bin/tensora export <id>
```

To try local accounts (sign-in and API tokens) instead of the dev header, keep the data in a SQLite file:

```bash
cd controlplane
export TENSORA_DATABASE_URL=sqlite:$PWD/tensora.db TENSORA_AUTH_MODE=local
go run ./cmd/tensora setup -email you@example.com   # once: asks for a password (12+ characters)
go run ./cmd/tensora dev                            # then `make web-dev`, open :5173, and sign in
```

On the dashboard's **API tokens** page, create a token for the CLI and set `TENSORA_TOKEN` to it. Forgot the
password? `go run ./cmd/tensora setup -reset-password -email you@example.com` sets a new one.

Sign-in works over HTTPS or from this machine. To sign in from another machine, put an HTTPS reverse proxy in
front and list its address in `TENSORA_TRUSTED_PROXIES`, or, only on a network you trust, set
`TENSORA_INSECURE_HTTP=true`.
