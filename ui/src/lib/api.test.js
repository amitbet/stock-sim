import { describe, expect, it } from "vitest";
import { retryAfterSeconds } from "./api.js";

describe("API rate limits", () => {
  it("supports Retry-After seconds and HTTP dates", () => {
    expect(retryAfterSeconds("12")).toBe(12);
    expect(retryAfterSeconds("Sat, 11 Jul 2026 09:01:30 GMT", Date.parse("2026-07-11T09:00:00Z"))).toBe(90);
  });

  it("returns null when Retry-After is unavailable", () => {
    expect(retryAfterSeconds("")).toBeNull();
    expect(retryAfterSeconds("not-a-date")).toBeNull();
  });
});
