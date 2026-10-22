# Object store (models, datasets, backups)

**Open decision for Phase 1 (ADR-0005).** Tensora needs S3-compatible storage for:

- tenant model weights, written by `tensora-ml ingest` and read by the KServe storage initializer
- backups (Velero, Postgres WAL archives)

| Option | Notes |
|---|---|
| **Rook-Ceph RGW** | The Phase 2+ answer. It comes with the Ceph cluster and needs ≥3 nodes to be worth it. |
| **SeaweedFS** | Light, Apache-2.0, and has a Helm chart. A reasonable single-node choice. |
| **Garage** | Light and built for small self-hosted clusters. AGPL. |
| **MinIO** | The community edition stopped shipping prebuilt binaries and images in late 2025 and moved to maintenance mode. Check its current status before choosing it. |

The object store is **regional**: every region runs its own, and model weights and backups for a
region stay in it (ADR-0009). `tensora-ml ingest` uploads to the store of the region where the model will be served.

Whichever you pick:

- Deploy it in the **`tensora-models`** namespace. cluster-config creates it with the label
  `tensora.io/role: model-store`, and tenant NetworkPolicies allow egress only to that namespace's
  pods, which is how the KServe storage initializer reaches the weights.
- Use one bucket per purpose (`tensora-models`, `tensora-backups`) and scope credentials per bucket.
- Add the release to `helmfile.yaml.gotmpl` under Layer 5, behind `features.objectStore`.
