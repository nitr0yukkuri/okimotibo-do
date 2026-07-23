import { describe, expect, it } from "vitest";
import { classifyLandmarks, normalizeMediaPipeGesture } from "./gesture-classifier";
import type { Landmark } from "./types";

function pose(thumb: "up" | "down" | "sideways", extendPinky: boolean): Landmark[] {
  const points = Array.from({ length: 21 }, () => ({ x: 0, y: 0.6, z: 0 }));
  points[0] = { x: 0, y: 1, z: 0 };
  if (thumb === "sideways") {
    points[1] = { x: 0.1, y: 0.8, z: 0 };
    points[2] = { x: 0.35, y: 0.75, z: 0 };
    points[3] = { x: 0.6, y: 0.74, z: 0 };
    points[4] = { x: 0.95, y: 0.73, z: 0 };
  } else {
    points[1] = { x: 0.2, y: thumb === "up" ? 0.9 : 0.8, z: 0 };
    points[2] = { x: 0.2, y: thumb === "up" ? 0.75 : 0.9, z: 0 };
    points[3] = { x: 0.2, y: thumb === "up" ? 0.5 : 1.1, z: 0 };
    points[4] = { x: 0.2, y: thumb === "up" ? 0.1 : 1.4, z: 0 };
  }

  const folded = (mcp: number, pip: number, tip: number, x: number) => {
    points[mcp] = { x, y: 0.7, z: 0 };
    points[pip] = { x, y: 0.45, z: 0 };
    points[tip] = { x: x + 0.12, y: 0.58, z: 0 };
  };
  folded(5, 6, 8, -0.3);
  folded(9, 10, 12, -0.1);
  folded(13, 14, 16, 0.15);
  if (extendPinky) {
    points[17] = { x: 0.4, y: 0.7, z: 0 };
    points[18] = { x: 0.4, y: 0.4, z: 0 };
    points[20] = { x: 0.4, y: 0, z: 0 };
  } else {
    folded(17, 18, 20, 0.4);
  }
  return points;
}

describe("gesture classifier", () => {
  it("normalizes only supported canned gestures", () => {
    expect(normalizeMediaPipeGesture("Thumb_Up")).toBe("thumb_up");
    expect(normalizeMediaPipeGesture("Thumb_Down")).toBe("thumb_down");
    expect(normalizeMediaPipeGesture("Open_Palm")).toBe("unknown");
  });

  it("rejects malformed landmark input", () => {
    expect(classifyLandmarks([])).toEqual({ gesture: "unknown", confidence: 0 });
  });

  it.each([
    ["thumb_up", pose("up", false)],
    ["sideways_thumb", pose("sideways", false)],
    ["shaka", pose("up", true)],
    ["thumb_down", pose("down", false)],
  ] as const)("recognizes %s from hand geometry", (expected, landmarks) => {
    expect(classifyLandmarks(landmarks).gesture).toBe(expected);
  });
});
