import { describe, expect, it } from "vitest";

import { filenameFromDisposition, toApiError, workloadPath } from "./api/client";
import { availableActions, classShown, describeChange, isSettling } from "./workloads";

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

  it("shows the request ID so users can find the exact log lines", () => {
    const err = toApiError(500, { error: { code: "internal", message: "internal error", request_id: "abc-123" } });
    expect(err.requestId).toBe("abc-123");
    expect(err.message).toContain("request ID abc-123");
    expect(toApiError(502, undefined, "from-header").requestId).toBe("from-header");
  });
});

describe("filenameFromDisposition", () => {
  it("uses the server's file name", () => {
    expect(filenameFromDisposition("attachment; filename=tensora-llm-1.json", "x.json")).toBe("tensora-llm-1.json");
    expect(filenameFromDisposition('attachment; filename="a b.json"', "x.json")).toBe("a b.json");
  });

  it("never lets the server choose a directory", () => {
    expect(filenameFromDisposition('attachment; filename="../../etc/evil.json"', "x.json")).toBe("evil.json");
  });

  it("falls back when there's no header", () => {
    expect(filenameFromDisposition(null, "fallback.json")).toBe("fallback.json");
  });
});

describe("workloadPath", () => {
  it("encodes the ID so it can't reach another API path", () => {
    expect(workloadPath("abc", "/events")).toBe("/v1/workloads/abc/events");
    expect(workloadPath("../../v1/catalog?", "/export")).toBe("/v1/workloads/..%2F..%2Fv1%2Fcatalog%3F/export");
  });
});

describe("describeChange", () => {
  it("shows state changes and notes", () => {
    expect(describeChange({ id: 1, to: "pending", reason: "created", at: "" })).toBe("∅ → pending");
    expect(describeChange({ id: 2, from: "pending", to: "queued", reason: "waiting", at: "" })).toBe("pending → queued");
    expect(describeChange({ id: 3, from: "pending", to: "pending", reason: "retrying", at: "" })).toBe("still pending");
  });
});

describe("filenameFromDisposition with bad encoding", () => {
  it("falls back instead of throwing", () => {
    expect(filenameFromDisposition("attachment; filename*=UTF-8''%E0%A4%A", "x.json")).toBe("x.json");
  });
});

describe("classShown", () => {
  it("shows what actually runs, and never hides a fallback", () => {
    expect(classShown({ gpu_class: "l40s-48gb", gpu_count: 2 })).toBe("2 × l40s-48gb");
    expect(classShown({ gpu_class: "l40s-48gb", gpu_count: 2, placed_class: "l40s-48gb" })).toBe("2 × l40s-48gb");
    expect(classShown({ gpu_class: "l40s-48gb", gpu_count: 1, placed_class: "cpu-16c-64g" })).toBe(
      "1 × cpu-16c-64g (requested l40s-48gb)",
    );
  });
});
