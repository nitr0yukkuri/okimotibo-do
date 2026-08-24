import { FaceLandmarker, FilesetResolver } from "@mediapipe/tasks-vision";
import { AdaptiveFaceClassifier } from "./face-classifier";
import type { RecognitionResult } from "./types";

export interface MediaPipeFaceOptions {
  enabled?: boolean;
  wasmRoot?: string;
  faceModelUrl?: string;
  inferenceIntervalMs?: number;
  smileHoldMs?: number;
  lockMs?: number;
  minConfidence?: number;
  busyMinConfidence?: number;
  delegate?: "CPU" | "GPU";
}

const DEFAULT_WASM = "https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@0.10.22-rc.20250304/wasm";
const DEFAULT_FACE_MODEL =
  "https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task";

export class SustainedSmileGate {
  private smileStartedAt: number | null = null;
  private lockedUntil = 0;
  private armed = true;

  constructor(
    private readonly holdMs = 2_000,
    private readonly lockMs = 4_000,
  ) {}

  push(smiling: boolean, now: number): boolean {
    if (!smiling) {
      this.smileStartedAt = null;
      if (now >= this.lockedUntil) this.armed = true;
      return false;
    }
    if (!this.armed || now < this.lockedUntil) return false;
    if (this.smileStartedAt === null) {
      this.smileStartedAt = now;
      return false;
    }
    if (now - this.smileStartedAt < this.holdMs) return false;

    this.smileStartedAt = null;
    this.lockedUntil = now + this.lockMs;
    this.armed = false;
    return true;
  }

  reset(): void {
    this.smileStartedAt = null;
    this.lockedUntil = 0;
    this.armed = true;
  }
}

type RecognizedExpression = "smile" | "frown";

export class SustainedExpressionGate {
  private expressionStartedAt: number | null = null;
  private candidate: RecognizedExpression | null = null;
  private lockedUntil = 0;
  private armed = true;

  constructor(
    private readonly holdMs = 2_000,
    private readonly lockMs = 4_000,
  ) {}

  push(expression: RecognizedExpression | null, now: number): RecognizedExpression | null {
    if (!expression) {
      this.expressionStartedAt = null;
      this.candidate = null;
      if (now >= this.lockedUntil) this.armed = true;
      return null;
    }
    if (!this.armed || now < this.lockedUntil) return null;
    if (this.candidate !== expression) {
      this.candidate = expression;
      this.expressionStartedAt = now;
      return null;
    }
    if (this.expressionStartedAt === null || now - this.expressionStartedAt < this.holdMs) return null;

    this.expressionStartedAt = null;
    this.candidate = null;
    this.lockedUntil = now + this.lockMs;
    this.armed = false;
    return expression;
  }

  reset(): void {
    this.expressionStartedAt = null;
    this.candidate = null;
    this.lockedUntil = 0;
    this.armed = true;
  }
}

export class MediaPipeFaceRecognizer {
  private faceLandmarker?: FaceLandmarker;
  private readonly classifier = new AdaptiveFaceClassifier({ calibrationSamples: 5 });
  private readonly gate: SustainedExpressionGate;
  private lastInferenceAt = 0;
  private readonly options: Required<Omit<MediaPipeFaceOptions, "enabled">>;

  constructor(options: MediaPipeFaceOptions = {}) {
    this.options = {
      wasmRoot: options.wasmRoot ?? DEFAULT_WASM,
      faceModelUrl: options.faceModelUrl ?? DEFAULT_FACE_MODEL,
      inferenceIntervalMs: options.inferenceIntervalMs ?? 100,
      smileHoldMs: options.smileHoldMs ?? 1_500,
      lockMs: options.lockMs ?? 4_000,
      minConfidence: options.minConfidence ?? 0.55,
      busyMinConfidence: options.busyMinConfidence ?? 0.35,
      delegate: options.delegate ?? "GPU",
    };
    this.gate = new SustainedExpressionGate(this.options.smileHoldMs, this.options.lockMs);
  }

  async initialize(): Promise<void> {
    const vision = await FilesetResolver.forVisionTasks(this.options.wasmRoot);
    this.faceLandmarker = await FaceLandmarker.createFromOptions(vision, {
      baseOptions: { modelAssetPath: this.options.faceModelUrl, delegate: this.options.delegate },
      runningMode: "VIDEO",
      numFaces: 1,
      minFaceDetectionConfidence: 0.6,
      minFacePresenceConfidence: 0.6,
      minTrackingConfidence: 0.6,
      outputFaceBlendshapes: true,
    });
  }

  recognize(video: HTMLVideoElement, timestampMs: number): RecognitionResult | null {
    if (!this.faceLandmarker) throw new Error("face recognizer is not initialized");
    if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) return null;
    if (timestampMs - this.lastInferenceAt < this.options.inferenceIntervalMs) return null;
    this.lastInferenceAt = timestampMs;

    const result = this.faceLandmarker.detectForVideo(video, timestampMs);
    const categories = result.faceBlendshapes[0]?.categories;
    if (!categories) {
      this.gate.push(null, timestampMs);
      return null;
    }

    const blendshapes = Object.fromEntries(categories.map(({ categoryName, score }) => [categoryName, score]));
    const classification = this.classifier.classify(blendshapes);
    let candidate: RecognizedExpression | null = null;
    if (classification.calibrated) {
      if (classification.expression === "smile" && classification.confidence >= this.options.minConfidence) {
        candidate = "smile";
      } else if (
        classification.expression === "frown" &&
        classification.confidence >= this.options.busyMinConfidence
      ) {
        candidate = "frown";
      }
    }
    const expression = this.gate.push(candidate, timestampMs);
    if (!expression) return null;

    return {
      capturedAt: new Date().toISOString(),
      hand: null,
      face: {
        expression,
        confidence: classification.confidence,
        blendshapes,
        calibrated: true,
        calibrationProgress: 1,
        scores: classification.scores,
      },
      status: expression === "smile" ? "available" : "busy",
      source: "face",
    };
  }

  close(): void {
    this.faceLandmarker?.close();
    this.faceLandmarker = undefined;
    this.classifier.reset();
    this.gate.reset();
  }
}
