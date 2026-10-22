# 0005. Use Longhorn and a small S3 store first, then Rook-Ceph from Phase 2

- **Status:** Accepted (the Phase 1 object store is still open)
- **Date:** 2026-10-22

## Context

We need block storage (VM disks, notebook volumes, Postgres) and S3 (models, backups), and later a shared
filesystem. Ceph does all three but wants at least 3 nodes and real operational effort. Phase 1 is one node.

## Decision

- **Phase 1–2:** Longhorn for block storage. For S3, a light store in `tensora-models`: SeaweedFS or Garage, decided during Phase 1. See `deploy/platform/object-store/README.md`.
- **Phase 2+** (at least 3 nodes): **Rook-Ceph** provides RBD (block), RGW (S3), and CephFS (shared). The `tensora-block` StorageClass switches provisioner, and the control plane doesn't change.
- MinIO, often the default suggestion, isn't chosen. Its community edition stopped shipping prebuilt artifacts in late 2025 and went into maintenance mode, so check its current status if reconsidering.

## Consequences

- One StorageClass name (`tensora-block`) hides the backend switch.
- Migrating volumes from Longhorn to Ceph needs a plan: copying with VM downtime, or letting customer workloads churn.
- On one node with one replica, a disk loss is data loss. Postgres backups must leave the node from day one.

## Alternatives considered

- **Ceph from day one:** possible on one node, but fragile and with no real redundancy.
- **local-path only:** fastest, but no replication, snapshots, or expansion.
- **NFS:** simple, but a single point of failure, and a poor fit for VM disks.
