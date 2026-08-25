import { describe, expect, it } from "vitest";
import { PrimaryHandSelector } from "./hand-selector";
import type { Landmark } from "./types";

function hand(width: number, height = width): Landmark[] {
  return Array.from({ length: 21 }, (_, index) => ({
    x: index % 2 === 0 ? 0 : width,
    y: index % 3 === 0 ? 0 : height,
    z: 0,
  }));
}

describe("PrimaryHandSelector", () => {
  it("selects the hand with the largest image-space area", () => {
    const selector = new PrimaryHandSelector();

    expect(selector.select([hand(0.2), hand(0.5)], 0)).toBe(1);
  });

  it("does not switch for a brief competing hand", () => {
    const selector = new PrimaryHandSelector({ switchDelayMs: 400 });
    const small = hand(0.2);
    const large = hand(0.5);

    expect(selector.select([small, []], 0)).toBe(0);
    expect(selector.select([small, large], 200)).toBe(0);
    expect(selector.select([small, large], 399)).toBe(0);
  });

  it("switches after the closer hand remains larger", () => {
    const selector = new PrimaryHandSelector({ switchDelayMs: 400 });
    const small = hand(0.2);
    const large = hand(0.5);

    expect(selector.select([small, []], 0)).toBe(0);
    expect(selector.select([small, large], 100)).toBe(0);
    expect(selector.select([small, large], 500)).toBe(1);
  });

  it("switches immediately when the active hand disappears", () => {
    const selector = new PrimaryHandSelector();
    const first = hand(0.3);
    const second = hand(0.2);

    expect(selector.select([first, second], 0)).toBe(0);
    expect(selector.select([[], second], 100)).toBe(1);
  });

  it("returns no hand when nothing is detected", () => {
    const selector = new PrimaryHandSelector();

    expect(selector.select([[], []], 0)).toBeUndefined();
  });
});
