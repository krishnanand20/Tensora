# Security

## Reporting a vulnerability

Email **security@tensora.example** (replace with a real address before launch). Do not open
public issues for vulnerabilities. We aim to acknowledge reports within 2 business days.

## Threat model in one paragraph

Tenants are untrusted and may be hostile. They run arbitrary code on our GPUs, upload
arbitrary model files, and probe the network. We must prevent them from:

- reading or affecting another tenant's data, GPUs, or traffic
- escaping to the host or the Kubernetes control plane
- using our infrastructure to attack third parties
- running compute that isn't billed

The full model and its controls are in
[docs/architecture/tenancy-and-isolation.md](docs/architecture/tenancy-and-isolation.md).

## Controls that must stay in place

- Untrusted workloads run in KubeVirt VMs or Kata containers, never in shared-kernel containers.
- Tenant namespaces have a default-deny NetworkPolicy that blocks private and link-local ranges.
- Tenant pods don't mount a ServiceAccount token. Tenants never get Kubernetes API access.
- Tenant models must be safetensors, and `trust_remote_code` is never enabled.
- The controller's ServiceAccount is the most powerful credential in the cluster. The API has no cluster access.
- No secrets in git. Use Ansible Vault for node secrets and Kubernetes Secrets, created out of band, for cluster secrets.

## Supported versions

Pre-release. Only `main` is supported.
