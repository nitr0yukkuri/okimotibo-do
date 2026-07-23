import { describe, expect, it } from "vitest";
import { AdaptiveFaceClassifier, classifyExpression } from "./face-classifier";

function calibratedClassifier(baseline: Record<string, number> = {}): AdaptiveFaceClassifier {
  const classifier = new AdaptiveFaceClassifier({ calibrationSamples: 5, baselineAdaptationRate: 0 });
  for (let index = 0; index < 5; index += 1) classifier.classify(baseline);
  return classifier;
}

describe("AdaptiveFaceClassifier", () => {
  it("learns the person's baseline before classifying", () => {
    const classifier = new AdaptiveFaceClassifier({ calibrationSamples: 5 });
    const result = classifier.classify({ mouthSmileLeft: 0.4, mouthSmileRight: 0.4 });
    expect(result.calibrated).toBe(false);
    expect(result.expression).toBe("neutral");
    expect(result.calibrationProgress).toBe(0.2);
  });

  it("classifies a bilateral smile relative to baseline", () => {
    const classifier = calibratedClassifier({ mouthSmileLeft: 0.2, mouthSmileRight: 0.2 });
    const result = classifier.classify({ mouthSmileLeft: 0.75, mouthSmileRight: 0.8 });
    expect(result.expression).toBe("smile");
    expect(result.scores.available).toBeGreaterThan(0.48);
  });

  it("does not call a single tense feature busy", () => {
    const classifier = calibratedClassifier();
    const result = classifier.classify({ browDownLeft: 0.9, browDownRight: 0.9 });
    expect(result.expression).toBe("neutral");
    expect(result.scores.busy).toBeLessThan(0.42);
  });

  it("combines brow, eye and mouth tension into a busy candidate", () => {
    const classifier = calibratedClassifier();
    const result = classifier.classify({
      browDownLeft: 0.7,
      browDownRight: 0.75,
      eyeSquintLeft: 0.65,
      eyeSquintRight: 0.6,
      mouthPressLeft: 0.7,
      mouthPressRight: 0.65,
    });
    expect(result.expression).toBe("frown");
    expect(result.scores.busy).toBeGreaterThanOrEqual(0.42);
  });

  it("keeps the stateless helper for previews", () => {
    expect(classifyExpression({ mouthSmileLeft: 0.8, mouthSmileRight: 0.9 }).expression).toBe("smile");
  });
});
