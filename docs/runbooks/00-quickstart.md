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

## Keep it private

`tensora up` listens only on localhost, and your machine is **private** until you choose otherwise.
To add more machines, or to share with others, see the Lab and sharing sections of the
[product spec](../product/product-spec.md#how-you-deploy-it).

## What works today

The development stack runs on macOS (Apple Silicon) and Linux. It uses a fake backend, so nothing
actually runs on your GPU yet:

```bash
make setup && make dev        # API on :8080
make web-dev                  # dashboard on :5173
```
