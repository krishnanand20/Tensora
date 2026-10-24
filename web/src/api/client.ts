import type { ApiErrorBody, Catalog, CreateWorkload, List, Workload, WorkloadEvent } from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function toApiError(status: number, body: unknown, requestId?: string | null): ApiError {
  const err = (body as Partial<ApiErrorBody> | undefined)?.error;
  const id = err?.request_id ?? requestId ?? undefined;
  const message = err?.message ?? `Request failed (HTTP ${status})`;
  // Show the request ID so users can hand it to an operator to find the exact log lines.
  return new ApiError(status, err?.code ?? "unknown", id ? `${message} (request ID ${id})` : message, id);
}

/** Reads the file name from a Content-Disposition header; never returns a path. */
export function filenameFromDisposition(header: string | null, fallback: string): string {
  const match = header?.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i);
  let name = fallback;
  try {
    name = match?.[1] ? decodeURIComponent(match[1]) : fallback;
  } catch {
    // Malformed percent-encoding: keep the fallback.
  }
  return name.split(/[\\/]/).pop() || fallback;
}

/** Path to one workload. The ID is encoded, so it can never point at a different API path. */
export function workloadPath(id: string, suffix = ""): string {
  return `/v1/workloads/${encodeURIComponent(id)}${suffix}`;
}

function authHeaders(): Record<string, string> {
  // Dev auth only. TODO(phase-1): local accounts and API tokens; OIDC with PKCE for the Datacenter profile (phase 3).
  const tenant = import.meta.env.VITE_DEV_TENANT ?? (import.meta.env.DEV ? "acme" : undefined);
  return tenant ? { "X-Dev-Tenant": tenant } : {};
}

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = authHeaders();
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw toApiError(res.status, text ? JSON.parse(text) : undefined, res.headers.get("X-Request-ID"));
  }
  return res;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const text = await (await send(method, path, body)).text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/** Downloads everything Tensora records about a workload, exactly as the server sent it. */
async function exportWorkload(id: string): Promise<{ blob: Blob; filename: string }> {
  const res = await send("GET", workloadPath(id, "/export"));
  return {
    blob: await res.blob(),
    filename: filenameFromDisposition(res.headers.get("Content-Disposition"), `tensora-workload-${id}.json`),
  };
}

export const api = {
  catalog: () => request<Catalog>("GET", "/v1/catalog"),
  /** includeDeleted adds deleted workloads, whose timeline and full log stay available (ADR-0015). */
  listWorkloads: (includeDeleted = false) =>
    request<List<Workload>>("GET", includeDeleted ? "/v1/workloads?include_deleted=true" : "/v1/workloads"),
  getWorkload: (id: string, includeDeleted = false) =>
    request<Workload>("GET", workloadPath(id, includeDeleted ? "?include_deleted=true" : "")),
  workloadEvents: (id: string) => request<List<WorkloadEvent>>("GET", workloadPath(id, "/events")),
  exportWorkload,
  createWorkload: (input: CreateWorkload) => request<Workload>("POST", "/v1/workloads", input),
  stopWorkload: (id: string) => request<Workload>("POST", workloadPath(id, "/stop")),
  startWorkload: (id: string) => request<Workload>("POST", workloadPath(id, "/start")),
  deleteWorkload: (id: string) => request<Workload>("DELETE", workloadPath(id)),
};
