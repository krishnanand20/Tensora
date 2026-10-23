import type { ApiErrorBody, Catalog, CreateWorkload, List, Workload } from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function toApiError(status: number, body: unknown): ApiError {
  const err = (body as Partial<ApiErrorBody> | undefined)?.error;
  return new ApiError(status, err?.code ?? "unknown", err?.message ?? `Request failed (HTTP ${status})`);
}

function authHeaders(): Record<string, string> {
  // Dev auth only. TODO(phase-1): local accounts and API tokens; OIDC with PKCE for the Datacenter profile (phase 3).
  const tenant = import.meta.env.VITE_DEV_TENANT ?? (import.meta.env.DEV ? "acme" : undefined);
  return tenant ? { "X-Dev-Tenant": tenant } : {};
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = authHeaders();
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data: unknown = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw toApiError(res.status, data);
  return data as T;
}

export const api = {
  catalog: () => request<Catalog>("GET", "/v1/catalog"),
  listWorkloads: () => request<List<Workload>>("GET", "/v1/workloads"),
  createWorkload: (input: CreateWorkload) => request<Workload>("POST", "/v1/workloads", input),
  stopWorkload: (id: string) => request<Workload>("POST", `/v1/workloads/${id}/stop`),
  startWorkload: (id: string) => request<Workload>("POST", `/v1/workloads/${id}/start`),
  deleteWorkload: (id: string) => request<Workload>("DELETE", `/v1/workloads/${id}`),
};
