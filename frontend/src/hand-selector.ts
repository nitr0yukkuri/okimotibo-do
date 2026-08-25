import type { Landmark } from "./types";

export interface PrimaryHandSelectorOptions {
  /** How long a larger competing hand must stay larger before switching. */
  switchDelayMs?: number;
  /** The new hand must be this much larger to replace the current hand. */
  switchAreaRatio?: number;
  /** Maximum normalized distance used to keep tracking the current hand. */
  matchDistance?: number;
}

interface HandCandidate {
  index: number;
  centerX: number;
  centerY: number;
  area: number;
}

interface PendingSwitch extends HandCandidate {
  since: number;
}

function measureHand(points: Landmark[], index: number): HandCandidate {
  if (points.length === 0) return { index, centerX: 0, centerY: 0, area: 0 };

  const xs = points.map((point) => point.x);
  const ys = points.map((point) => point.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);

  return {
    index,
    centerX: (minX + maxX) / 2,
    centerY: (minY + maxY) / 2,
    area: Math.max(0, maxX - minX) * Math.max(0, maxY - minY),
  };
}

function distance(a: HandCandidate, b: HandCandidate): number {
  return Math.hypot(a.centerX - b.centerX, a.centerY - b.centerY);
}

function largest(candidates: HandCandidate[]): HandCandidate {
  return candidates.reduce((best, candidate) => candidate.area > best.area ? candidate : best);
}

/**
 * Chooses one representative hand when MediaPipe detects multiple hands.
 * A larger image-space hand is treated as the hand closest to the camera.
 */
export class PrimaryHandSelector {
  private readonly options: Required<PrimaryHandSelectorOptions>;
  private active?: HandCandidate;
  private pendingSwitch?: PendingSwitch;

  constructor(options: PrimaryHandSelectorOptions = {}) {
    this.options = {
      switchDelayMs: options.switchDelayMs ?? 400,
      switchAreaRatio: options.switchAreaRatio ?? 1.2,
      matchDistance: options.matchDistance ?? 0.24,
    };
  }

  select(hands: Landmark[][], now = Date.now()): number | undefined {
    const candidates = hands
      .map((points, index) => measureHand(points, index))
      .filter((candidate) => candidate.area > 0);

    if (candidates.length === 0) {
      this.active = undefined;
      this.pendingSwitch = undefined;
      return undefined;
    }

    const nearest = largest(candidates);
    if (!this.active) {
      this.active = nearest;
      return nearest.index;
    }

    const current = candidates
      .map((candidate) => ({ candidate, distance: distance(candidate, this.active!) }))
      .sort((a, b) => a.distance - b.distance)[0];

    if (!current || current.distance > this.options.matchDistance) {
      this.active = nearest;
      this.pendingSwitch = undefined;
      return nearest.index;
    }

    this.active = current.candidate;
    if (nearest.index === current.candidate.index || nearest.area < current.candidate.area * this.options.switchAreaRatio) {
      this.pendingSwitch = undefined;
      return current.candidate.index;
    }

    if (!this.pendingSwitch || distance(this.pendingSwitch, nearest) > this.options.matchDistance) {
      this.pendingSwitch = { ...nearest, since: now };
      return current.candidate.index;
    }

    this.pendingSwitch = { ...nearest, since: this.pendingSwitch.since };
    if (now - this.pendingSwitch.since < this.options.switchDelayMs) return current.candidate.index;

    this.active = nearest;
    this.pendingSwitch = undefined;
    return nearest.index;
  }
}
