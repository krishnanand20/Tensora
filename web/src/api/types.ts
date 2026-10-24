// Mirrors controlplane/internal/api/json.go. Change both together.

export type WorkloadKind = "vm" | "notebook" | "endpoint";

export type WorkloadState =
  | "pending"
  | "queued"
  | "provisioning"
  | "running"
  | "stopping"
  | "stopped"
  | "failed"
  | "deleting"
  | "deleted";

export interface Workload {
  id: string;
  name: string;
  kind: WorkloadKind;
  state: WorkloadState;
  image_id: string;
  gpu_class: string;
  gpu_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateWorkload {
  name: string;
  kind: WorkloadKind;
  image_id: string;
  gpu_class: string;
  gpu_count: number;
  ssh_public_key?: string;
  model_uri?: string;
}

export interface GpuClass {
  id: string;
  display_name: string;
  vram_gb: number;
  supports_vm: boolean;
}

export interface CatalogImage {
  id: string;
  display_name: string;
  kind: WorkloadKind;
  description: string;
}

export interface Catalog {
  gpu_classes: GpuClass[];
  images: CatalogImage[];
}

export interface List<T> {
  items: T[];
}

/** One entry in a workload's timeline: a state change, or a note when `from` equals `to` (ADR-0015). */
export interface WorkloadEvent {
  id: number;
  from?: WorkloadState;
  to: WorkloadState;
  reason: string;
  at: string;
}

export interface ApiErrorBody {
  /** request_id matches the X-Request-ID header and the operator's log lines. */
  error: { code: string; message: string; request_id?: string };
}
