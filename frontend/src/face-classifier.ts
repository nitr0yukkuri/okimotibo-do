import type { Expression } from "./types";

export interface FaceSignalScores {
  available: number;
  busy: number;
  surprised: number;
  browTension: number;
  eyeTension: number;
  mouthTension: number;
}

export interface ExpressionClassification {
  expression: Expression;
  confidence: number;
  calibrated: boolean;
  calibrationProgress: number;
  scores: FaceSignalScores;
}

export interface AdaptiveFaceClassifierOptions {
  calibrationSamples?: number;
  baselineAdaptationRate?: number;
}

const FEATURE_NAMES = [
  "mouthSmileLeft", "mouthSmileRight", "cheekSquintLeft", "cheekSquintRight",
  "mouthFrownLeft", "mouthFrownRight", "mouthPressLeft", "mouthPressRight",
  "browDownLeft", "browDownRight", "eyeSquintLeft", "eyeSquintRight",
  "jawOpen", "eyeWideLeft", "eyeWideRight", "browInnerUp",
  "browOuterUpLeft", "browOuterUpRight",
] as const;

const clamp = (value: number): number => Math.max(0, Math.min(1, value));
const average = (values: number[]): number => values.reduce((sum, value) => sum + value, 0) / values.length;

function emptyScores(): FaceSignalScores {
  return { available: 0, busy: 0, surprised: 0, browTension: 0, eyeTension: 0, mouthTension: 0 };
}

export class AdaptiveFaceClassifier {
  private readonly calibrationSamples: number;
  private readonly baselineAdaptationRate: number;
  private readonly baseline: Record<string, number> = {};
  private samples = 0;

  constructor(options: AdaptiveFaceClassifierOptions = {}) {
    this.calibrationSamples = Math.max(5, options.calibrationSamples ?? 30);
    this.baselineAdaptationRate = clamp(options.baselineAdaptationRate ?? 0.005);
  }

  classify(shapes: Record<string, number>): ExpressionClassification {
    const values = Object.fromEntries(FEATURE_NAMES.map((name) => [name, clamp(shapes[name] ?? 0)]));

    if (this.samples < this.calibrationSamples) {
      this.samples += 1;
      for (const name of FEATURE_NAMES) {
        const previous = this.baseline[name] ?? 0;
        this.baseline[name] = previous + (values[name] - previous) / this.samples;
      }
      return {
        expression: "neutral",
        confidence: 1,
        calibrated: this.samples >= this.calibrationSamples,
        calibrationProgress: this.samples / this.calibrationSamples,
        scores: emptyScores(),
      };
    }

    const delta = (name: string, deadZone = 0.04, range = 0.35): number =>
      clamp((values[name] - (this.baseline[name] ?? 0) - deadZone) / range);
    const pair = (left: string, right: string): number => average([delta(left), delta(right)]);

    const smileMouth = pair("mouthSmileLeft", "mouthSmileRight");
    const smileEyes = pair("cheekSquintLeft", "cheekSquintRight");
    const browTension = pair("browDownLeft", "browDownRight");
    const eyeTension = pair("eyeSquintLeft", "eyeSquintRight");
    const mouthPress = pair("mouthPressLeft", "mouthPressRight");
    const mouthFrown = pair("mouthFrownLeft", "mouthFrownRight");
    const mouthTension = 0.6 * mouthPress + 0.4 * mouthFrown;
    const surpriseEyes = pair("eyeWideLeft", "eyeWideRight");
    const surpriseBrows = average([
      delta("browInnerUp"),
      delta("browOuterUpLeft"),
      delta("browOuterUpRight"),
    ]);

    const available = clamp(0.75 * smileMouth + 0.25 * smileEyes);
    let busy = clamp(0.32 * browTension + 0.22 * eyeTension + 0.28 * mouthPress + 0.18 * mouthFrown);
    const busyEvidence = [browTension, eyeTension, mouthPress, mouthFrown].filter((value) => value >= 0.2).length;
    if (busyEvidence < 2) busy *= 0.4;
    const surprised = clamp(0.35 * delta("jawOpen") + 0.3 * surpriseEyes + 0.35 * surpriseBrows);
    const scores = { available, busy, surprised, browTension, eyeTension, mouthTension };

    let expression: Expression = "neutral";
    let confidence = 1 - Math.max(available, busy, surprised);
    if (available >= 0.48 && available >= busy && available >= surprised) {
      expression = "smile";
      confidence = available;
    } else if (busy >= 0.42 && busy >= surprised) {
      // API名は後方互換のためfrownだが、複数特徴による「集中傾向」を表す。
      expression = "frown";
      confidence = busy;
    } else if (surprised >= 0.55) {
      expression = "surprised";
      confidence = surprised;
    } else {
      this.adaptBaseline(values);
    }

    return { expression, confidence: clamp(confidence), calibrated: true, calibrationProgress: 1, scores };
  }

  reset(): void {
    this.samples = 0;
    for (const name of FEATURE_NAMES) delete this.baseline[name];
  }

  private adaptBaseline(values: Record<string, number>): void {
    for (const name of FEATURE_NAMES) {
      this.baseline[name] += (values[name] - this.baseline[name]) * this.baselineAdaptationRate;
    }
  }
}

// Stateless compatibility helper for callers that only need a single-frame preview.
export function classifyExpression(shapes: Record<string, number>): ExpressionClassification {
  const classifier = new AdaptiveFaceClassifier({ calibrationSamples: 5 });
  for (let index = 0; index < 5; index += 1) classifier.classify({});
  return classifier.classify(shapes);
}
