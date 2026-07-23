import { describe, expect, it } from "vitest";
import { TemporalStabilizer } from "./stabilizer";

describe("TemporalStabilizer", () => {
  it("requires repeated high-confidence samples", () => {
    const stabilizer = new TemporalStabilizer<"unknown" | "up">("unknown", { windowSize: 4, requiredMatches: 3, minimumConfidence: 0.7 });
    expect(stabilizer.push("up", 0.9, 100)).toBe("unknown");
    expect(stabilizer.push("up", 0.9, 110)).toBe("unknown");
    expect(stabilizer.push("up", 0.9, 120)).toBe("up");
  });

  it("keeps the last state during brief detection loss", () => {
    const stabilizer = new TemporalStabilizer<"unknown" | "up">("unknown", { windowSize: 2, requiredMatches: 1, missingGraceMs: 1000 });
    expect(stabilizer.push("up", 1, 100)).toBe("up");
    expect(stabilizer.push("unknown", 0, 500)).toBe("up");
    expect(stabilizer.push("unknown", 0, 1100)).toBe("unknown");
  });
});
