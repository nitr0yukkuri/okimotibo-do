import {
  FilesetResolver,
  GestureRecognizer,
  type NormalizedLandmark,
} from "@mediapipe/tasks-vision";
import { classifyLandmarks, normalizeMediaPipeGesture } from "./gesture-classifier";
import { TemporalStabilizer } from "./stabilizer";
import {
  GESTURE_STATUS,
  type Gesture,
  type Landmark,
  type RecognitionResult,
} from "./types";

export interface RecognizerOptions {
  wasmRoot?: string;
  gestureModelUrl?: string;
  inferenceIntervalMs?: number;
  minConfidence?: number;
  delegate?: "CPU" | "GPU";
}

const DEFAULT_WASM = "https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@0.10.22-rc.20250304/wasm";
const DEFAULT_GESTURE_MODEL =
  "https://storage.googleapis.com/mediapipe-models/gesture_recognizer/gesture_recognizer/float16/1/gesture_recognizer.task";

function toLandmarks(points: NormalizedLandmark[]): Landmark[] {
  return points.map(({ x, y, z }) => ({ x, y, z }));
}

export class MediaPipeStateRecognizer {
  private gestureRecognizer?: GestureRecognizer;
  private readonly stabilizer: TemporalStabilizer<Gesture>;
  private lastInferenceAt = 0;
  private readonly options: Required<RecognizerOptions>;

  constructor(options: RecognizerOptions = {}) {
    this.options = {
      wasmRoot: options.wasmRoot ?? DEFAULT_WASM,
      gestureModelUrl: options.gestureModelUrl ?? DEFAULT_GESTURE_MODEL,
      inferenceIntervalMs: options.inferenceIntervalMs ?? 100,
      minConfidence: options.minConfidence ?? 0.7,
      delegate: options.delegate ?? "GPU",
    };
    if (!Number.isFinite(this.options.inferenceIntervalMs) || this.options.inferenceIntervalMs <= 0) {
      throw new Error("inferenceIntervalMs must be positive");
    }
    if (this.options.minConfidence < 0 || this.options.minConfidence > 1) {
      throw new Error("minConfidence must be between 0 and 1");
    }
    this.stabilizer = new TemporalStabilizer<Gesture>("unknown", {
      minimumConfidence: this.options.minConfidence,
    });
  }

  async initialize(): Promise<void> {
    const vision = await FilesetResolver.forVisionTasks(this.options.wasmRoot);
    const baseOptions = { delegate: this.options.delegate } as const;
    this.gestureRecognizer = await GestureRecognizer.createFromOptions(vision, {
      baseOptions: { ...baseOptions, modelAssetPath: this.options.gestureModelUrl },
      runningMode: "VIDEO",
      numHands: 1,
      minHandDetectionConfidence: 0.6,
      minTrackingConfidence: 0.6,
    });
  }

  recognize(video: HTMLVideoElement, timestampMs = performance.now()): RecognitionResult | null {
    if (!this.gestureRecognizer) throw new Error("recognizer is not initialized");
    if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) return null;
    if (timestampMs - this.lastInferenceAt < this.options.inferenceIntervalMs) return null;
    this.lastInferenceAt = timestampMs;

    const handResult = this.gestureRecognizer.recognizeForVideo(video, timestampMs);
    const landmarks = handResult.landmarks[0] ? toLandmarks(handResult.landmarks[0]) : [];
    const custom = classifyLandmarks(landmarks);
    const canned = handResult.gestures[0]?.[0];
    const cannedGesture = normalizeMediaPipeGesture(canned?.categoryName);
    const useCanned = cannedGesture !== "unknown" && (canned?.score ?? 0) >= this.options.minConfidence;
    const customGesture = custom.gesture === "shaka" || custom.gesture === "sideways_thumb";
    const rawGesture = customGesture ? custom.gesture : useCanned ? cannedGesture : custom.gesture;
    const rawConfidence = customGesture || !useCanned ? custom.confidence : (canned?.score ?? 0);
    const stableGesture = this.stabilizer.push(rawGesture, rawConfidence, Date.now());
    if (stableGesture === "unknown") return null;

    return {
      capturedAt: new Date().toISOString(),
      hand: {
        gesture: stableGesture,
        confidence: rawConfidence,
        handedness: handResult.handedness[0]?.[0]?.categoryName as "Left" | "Right" | undefined,
      },
      face: null,
      status: GESTURE_STATUS[stableGesture],
      source: "hand",
    };
  }

  close(): void {
    this.gestureRecognizer?.close();
    this.gestureRecognizer = undefined;
  }
}
