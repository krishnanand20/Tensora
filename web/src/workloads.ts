import type { WorkloadState } from "./api/types";

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
