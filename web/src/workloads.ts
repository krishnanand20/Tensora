import type { WorkloadEvent, WorkloadState } from "./api/types";

export type Action = "stop" | "start" | "delete";

/** Which buttons to show. Mirrors the checks in controlplane/internal/service/service.go. */
export function availableActions(state: WorkloadState): Action[] {
  const actions: Action[] = [];
  if (state === "provisioning" || state === "running") actions.push("stop");
  if (state === "stopped" || state === "failed") actions.push("start");
  if (state !== "deleting" && state !== "deleted") actions.push("delete");
  return actions;
}

/** States where the controller is still working, so the list should poll. */
export function isSettling(state: WorkloadState): boolean {
  return ["pending", "queued", "provisioning", "stopping", "deleting"].includes(state);
}

/** How a timeline entry reads: a state change, or a note that leaves the state as it was. */
export function describeChange(e: WorkloadEvent): string {
  if (e.from === e.to) return `still ${e.to}`;
  return `${e.from ?? "∅"} → ${e.to}`;
}
