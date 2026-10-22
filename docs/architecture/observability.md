# Observability

## Stack

| Signal | Tool | Notes |
|---|---|---|
| Metrics | kube-prometheus-stack (Prometheus, Alertmanager, Grafana) | ServiceMonitors from every chart are picked up |
| GPU metrics | NVIDIA DCGM exporter (via GPU Operator) | Container-mode nodes only. Passthrough GPUs are invisible to the host. |
| Logs | Loki (single binary) + Grafana Alloy | Pod logs with `namespace`, `pod`, `container`, and `tenant` labels |
| Network flows | Cilium Hubble | Useful for policy debugging and abuse detection |
| Hardware | IPMI/BMC (power, temperature, fans, PSU) | Exporter or BMC SNMP, Phase 2 |

## Alerts that matter

| Alert | Source | Why |
|---|---|---|
| GPU XID errors (`DCGM_FI_DEV_XID_ERRORS` > 0) | DCGM | XID 79 (the GPU fell off the bus) or 48 (double-bit ECC) means draining the node and possibly an RMA |
| ECC double-bit errors / retired pages | DCGM | Hardware degradation |
| GPU temperature or power at its limit for 10 min or more | DCGM, IPMI | Cooling problems, which are common in Indian summers |
| Node NotReady, or disk over 85% | kube-state-metrics, node-exporter | |
| Longhorn or Ceph degraded | Their exporters | Data at risk |
| Workloads in `provisioning` longer than 15 min, or queue depth growing | Control plane `/metrics` (Phase 3) | Stuck admission or capacity exhaustion |
| Metering lag (last closed hour not metered after 30 min) | Control plane `/metrics` | Billing correctness |
| Certificate expiry within 14 days | cert-manager | |
| Postgres down or replication lag | Postgres exporter | |

## Dashboards

- NVIDIA DCGM exporter dashboard (Grafana.com dashboard 12239)
- Cilium and Hubble, Longhorn or Ceph, and node exporter dashboards come from their charts
- Tensora (Phase 3): workloads by state, queue depth by GPU class, GPUs allocated versus capacity, and revenue-hours per day

## Customer data in logs

Tenant container logs are **customer data**:

- Only on-call staff should have access to Loki, for operational reasons, and that access should be audited.
- Retention is 14 days (`loki.yaml`).
- Logs from inside VMs are never collected.
- Tenant-facing logs and metrics are Phase 4: per-workload log streaming via the API, with tenant-filtered queries.
