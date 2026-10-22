import { describe, expect, it } from "vitest";

import { toApiError } from "./api/client";
import { availableActions, isSettling } from "./workloads";

describe("availableActions", () => {
  it("matches the control plane's lifecycle rules", () => {
    expect(availableActions("running")).toEqual(["stop", "delete"]);
    expect(availableActions("stopped")).toEqual(["start", "delete"]);
    expect(availableActions("failed")).toEqual(["start", "delete"]);
    expect(availableActions("pending")).toEqual(["delete"]);
    expect(availableActions("deleting")).toEqual([]);
  });
});

describe("isSettling", () => {
  it("polls only while the controller is working", () => {
    expect(isSettling("provisioning")).toBe(true);
    expect(isSettling("running")).toBe(false);
  });
});

describe("toApiError", () => {
  it("reads the API error envelope", () => {
    const err = toApiError(409, { error: { code: "quota_exceeded", message: "l40s quota" } });
    expect(err.code).toBe("quota_exceeded");
    expect(err.message).toBe("l40s quota");
  });

  it("falls back for non-API responses", () => {
    expect(toApiError(502, undefined).message).toContain("502");
  });
});
