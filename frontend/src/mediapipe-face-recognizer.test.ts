import { describe, expect, it } from "vitest";
import { SustainedExpressionGate, SustainedSmileGate } from "./mediapipe-face-recognizer";

describe("SustainedSmileGate", () => {
  it("accepts a smile only after it continues for two seconds", () => {
    const gate = new SustainedSmileGate(2_000, 4_000);

    expect(gate.push(true, 1_000)).toBe(false);
    expect(gate.push(true, 2_999)).toBe(false);
    expect(gate.push(true, 3_000)).toBe(true);
  });

  it("resets the hold when the smile stops", () => {
    const gate = new SustainedSmileGate(2_000, 4_000);

    expect(gate.push(true, 1_000)).toBe(false);
    expect(gate.push(false, 2_000)).toBe(false);
    expect(gate.push(true, 3_000)).toBe(false);
    expect(gate.push(true, 5_000)).toBe(true);
  });

  it("requires a release after the lock before accepting another smile", () => {
    const gate = new SustainedSmileGate(2_000, 4_000);

    gate.push(true, 1_000);
    expect(gate.push(true, 3_000)).toBe(true);
    expect(gate.push(true, 8_000)).toBe(false);
    expect(gate.push(false, 8_001)).toBe(false);
    expect(gate.push(true, 8_002)).toBe(false);
    expect(gate.push(true, 10_002)).toBe(true);
  });
});

describe("SustainedExpressionGate", () => {
  it("accepts an angry expression only after it continues for two seconds", () => {
    const gate = new SustainedExpressionGate(2_000, 4_000);

    expect(gate.push("frown", 1_000)).toBeNull();
    expect(gate.push("frown", 2_999)).toBeNull();
    expect(gate.push("frown", 3_000)).toBe("frown");
  });

  it("restarts the hold when the expression changes", () => {
    const gate = new SustainedExpressionGate(2_000, 4_000);

    expect(gate.push("smile", 1_000)).toBeNull();
    expect(gate.push("frown", 2_000)).toBeNull();
    expect(gate.push("frown", 3_999)).toBeNull();
    expect(gate.push("frown", 4_000)).toBe("frown");
  });

  it("shares the lock between smile and angry expressions", () => {
    const gate = new SustainedExpressionGate(2_000, 4_000);

    gate.push("smile", 1_000);
    expect(gate.push("smile", 3_000)).toBe("smile");
    expect(gate.push("frown", 8_000)).toBeNull();
    expect(gate.push(null, 8_001)).toBeNull();
    expect(gate.push("frown", 8_002)).toBeNull();
    expect(gate.push("frown", 10_002)).toBe("frown");
  });
});
