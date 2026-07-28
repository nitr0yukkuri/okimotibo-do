export interface StabilizerOptions {
  windowSize?: number;
  requiredMatches?: number;
  minimumConfidence?: number;
  missingGraceMs?: number;
}

export class TemporalStabilizer<T extends string> {
  private readonly samples: Array<{ value: T; confidence: number }> = [];
  private stableValue: T;
  private lastSeenAt = 0;
  private readonly options: Required<StabilizerOptions>;

  constructor(private readonly unknownValue: T, options: StabilizerOptions = {}) {
    this.stableValue = unknownValue;
    this.options = {
      windowSize: options.windowSize ?? 8,
      requiredMatches: options.requiredMatches ?? 6,
      minimumConfidence: options.minimumConfidence ?? 0.7,
      missingGraceMs: options.missingGraceMs ?? 1500,
    };
    if (this.options.requiredMatches > this.options.windowSize) {
      throw new Error("requiredMatches must not exceed windowSize");
    }
  }

  push(value: T, confidence: number, now = Date.now()): T {
    if (value === this.unknownValue || confidence < this.options.minimumConfidence) {
      if (this.lastSeenAt > 0 && now - this.lastSeenAt >= this.options.missingGraceMs) {
        this.samples.length = 0;
        this.stableValue = this.unknownValue;
      }
      return this.stableValue;
    }

    this.lastSeenAt = now;
    this.samples.push({ value, confidence });
    if (this.samples.length > this.options.windowSize) this.samples.shift();

    const counts = new Map<T, number>();
    for (const sample of this.samples) {
      counts.set(sample.value, (counts.get(sample.value) ?? 0) + 1);
    }
    const winner = [...counts.entries()].sort((a, b) => b[1] - a[1])[0];
    if (winner && winner[1] >= this.options.requiredMatches) this.stableValue = winner[0];
    return this.stableValue;
  }

  current(): T {
    return this.stableValue;
  }
}

