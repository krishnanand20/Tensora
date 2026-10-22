# Storage

## Classes

| Need | Phase 1–2 | Phase 2+ | Referenced as |
|---|---|---|---|
| Block (VM disks, notebook work dirs, Postgres, Prometheus) | Longhorn | Ceph RBD | StorageClass **`tensora-block`** (cluster-config) |
| Object (models, datasets, backups) | Open: SeaweedFS or Garage ([object-store](../../deploy/platform/object-store/README.md)) | Ceph RGW | S3 endpoint in `tensora-models` |
| Shared filesystem (datasets mounted by many pods) | none | CephFS | future StorageClass `tensora-shared` |
| Local scratch (fast, ephemeral) | `emptyDir` on NVMe | TopoLVM or local-path for large scratch | future |

The control plane only knows `tensora-block` (`TENSORA_STORAGE_CLASS`), so moving from Longhorn
to Ceph changes `deploy/`, not code.

## VM images

Base images are CDI `DataSource`s in `tensora-images`. Each VM's root disk is a clone, created by
`dataVolumeTemplates` in the VM manifest. Cloning across namespaces is authorized by the
`tensora-image-cloner` ClusterRole. See [images/vm/](../../images/vm/ubuntu-24.04-cuda/README.md).

## Model weights

- Phase 1: the KServe storage initializer downloads from `hf://` or `s3://` every time an endpoint starts. This is slow for large models.
- Phase 3: `tensora-ml ingest` copies validated models into the object store once. Endpoints then pull from in-cluster S3 at 25 GbE speeds. Later, a node-local cache for popular models.

## Durability and backups

| Data | Protection | Phase |
|---|---|---|
| Postgres (tenants, usage, events) | **Critical.** Continuous WAL archiving to the object store plus daily base backups (CloudNativePG or pgBackRest). Test restores monthly. | 2 |
| etcd (cluster state) | `k3s etcd-snapshot` scheduled to S3 | 2 |
| Cluster objects | Velero, or rely on helmfile and the control plane re-applying them | 2 |
| Customer volumes | **Not backed up by default.** Stated in the ToS. Snapshots offered as a paid feature later. | 4 |

On a **single node with `replicas: 1`**, a disk failure loses customer data. Make that explicit
to any early users, and keep Postgres backups off-node from day one.

## Regions

- **Each regional cell has its own block storage and object store.** Workload data (disks, models, endpoint logs) never leaves its region unless the customer moves it ([ADR-0009](../adr/0009-regions-global-control-plane-regional-cells.md)).
- **Global data** (accounts, intent, events, usage) lives in the global Postgres. Its backups go to a location chosen for its privacy-law position, which must be documented in the privacy policy.
- A region's backups stay in that region, or in another facility in the same country.

## Encryption

Volumes are not encrypted at rest yet. Freed blocks can hold previous tenants' data. Plan:
Longhorn encrypted StorageClass or Ceph RBD encryption with per-volume keys (Phase 4).
