# Runbooks

Step-by-step procedures for operating real hardware. These are for **humans**: agents may
help draft and review them, but they never execute steps that change hosts or clusters.

| Runbook | When |
|---|---|
| [01-node-bootstrap.md](01-node-bootstrap.md) | A new server, from bare metal to a Ready k3s node with the platform installed |
| [02-gpu-passthrough.md](02-gpu-passthrough.md) | GPU smoke test, burn-in, then the first passthrough VM |
| [03-first-model-endpoint.md](03-first-model-endpoint.md) | The first vLLM endpoint through KServe |
| [add-node.md](add-node.md) | Adding capacity to an existing cluster |
| [incident-response.md](incident-response.md) | Something is broken |

Every runbook lists the expected output of each step. If reality differs, stop and fix the
runbook before continuing.
