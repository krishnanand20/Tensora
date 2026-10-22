# Incident response

## Severity

| Sev | Meaning | Response |
|---|---|---|
| **1** | Customer data at risk, a cross-tenant exposure, or the whole platform down | Page now; all hands; customer comms within 1 hour |
| **2** | One node or one workload kind down; billing wrong | Page during waking hours; fix the same day |
| **3** | Degraded but working (slow admission, one GPU lost) | Ticket; fix this week |

## First five minutes

1. **Acknowledge** and post in the incident channel: what you see, the start time, and the severity.
2. **Stop the bleeding** before diagnosing: cordon a sick node, pause the controller (scale it to 0) if it's doing damage, or block a single abusive tenant.
3. **Preserve evidence:** logs, `kubectl get events`, and DCGM history. Don't delete broken objects yet.
4. Keep a timeline as you go, with timestamps.

## Common incidents

| Incident | Detect | Action |
|---|---|---|
| **GPU XID 79** (fell off the bus) | DCGM alert, `dmesg` | Cordon, drain, and reboot the node. If it repeats, the GPU or riser is failing: take it out of service and RMA it. |
| **XID 48 / ECC double-bit** | DCGM | Drain; check retired pages; RMA if it recurs |
| **Node NotReady** | Alert | BMC console: power, kernel panic, or network? Workloads on it go to `failed` via the controller; notify the affected tenants. |
| **Disk full** | Alert at 85% | Prometheus and Loki retention, container images (`crictl rmi --prune`), Longhorn snapshots |
| **Workloads stuck in provisioning** | Alert, dashboard | `kubectl -n t-<tenant> describe vm|sts|isvc`; check controller logs; capacity versus allocated |
| **Postgres down** | readyz 503 | The API fails closed and running workloads continue. Restore from the replica or backup. |
| **Certificate expired** | Alert at 14 days | `kubectl get certificate -A`; check the ACME HTTP-01 route through the Gateway |
| **Tenant abuse** (mining stolen cards, outbound attacks, illegal content) | Abuse report, Hubble flows | Stop the tenant's workloads (API or DB), keep their data and logs, suspend the account, follow the AUP process, and get legal advice if needed |
| **Suspected cross-tenant exposure** | Anything | **Sev 1.** Stop affected workloads, preserve evidence, and investigate before restoring service. There may be a legal duty to notify (DPDP Act). |

## After every Sev 1 or Sev 2

Within 5 working days, write a blameless postmortem: timeline, impact (tenants and GPU-hours),
root cause, what went well, and action items with owners. Turn every new failure mode into
an alert, a runbook step, or a test.
