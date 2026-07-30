import type { Expression, RecognitionResult, Status } from "./types";

export interface EmotionApiOptions {
  url: string;
  enabled?: boolean;
  inferenceIntervalMs?: number;
  requestTimeoutMs?: number;
  windowSize?: number;
  requiredMatches?: number;
}

interface EmotionApiResponse {
  emotion: string;
  confidence: number;
  status: Status;
  scores: Record<string, number>;
}

function isStatus(value: unknown): value is Status {
  return value === "available" || value === "neutral" || value === "busy" || value === "unknown";
}

function expressionForEmotion(emotion: string): Expression {
  if (emotion === "Happy") return "smile";
  if (emotion === "Anger") return "frown";
  if (emotion === "Surprise") return "surprised";
  return "neutral";
}

export class EmotionApiClient {
  private readonly canvas = document.createElement("canvas");
  private readonly samples: Status[] = [];
  private lastInferenceAt = 0;
  private requestInFlight = false;
  private stableStatus: Status | null = null;

  constructor(private readonly options: EmotionApiOptions) {}

  async recognize(video: HTMLVideoElement, timestampMs: number): Promise<RecognitionResult | null> {
    const interval = this.options.inferenceIntervalMs ?? 1_000;
    if (
      this.requestInFlight ||
      timestampMs - this.lastInferenceAt < interval ||
      video.readyState < 2 ||
      video.videoWidth === 0 ||
      video.videoHeight === 0
    ) return null;

    this.lastInferenceAt = timestampMs;
    this.requestInFlight = true;
    try {
      const image = await this.capture(video);
      const response = await this.fetchPrediction(image);
      const stableStatus = this.push(response.status);
      if (!stableStatus) return null;

      return {
        capturedAt: new Date().toISOString(),
        hand: null,
        face: {
          expression: expressionForEmotion(response.emotion),
          confidence: response.confidence,
          blendshapes: {},
          calibrated: true,
          calibrationProgress: 1,
          scores: {
            available: response.scores.Happy ?? 0,
            busy: response.scores.Anger ?? 0,
            surprised: response.scores.Surprise ?? 0,
            browTension: 0,
            eyeTension: 0,
            mouthTension: 0,
          },
        },
        status: stableStatus,
        source: "face",
      };
    } finally {
      this.requestInFlight = false;
    }
  }

  reset(): void {
    this.samples.length = 0;
    this.stableStatus = null;
  }

  private push(status: Status): Status | null {
    if (status === "unknown") return null;
    const windowSize = this.options.windowSize ?? 5;
    const requiredMatches = this.options.requiredMatches ?? 3;
    this.samples.push(status);
    if (this.samples.length > windowSize) this.samples.shift();

    const matches = this.samples.filter((sample) => sample === status).length;
    if (matches < requiredMatches) return null;
    this.stableStatus = status;
    return status;
  }

  private async capture(video: HTMLVideoElement): Promise<Blob> {
    const width = Math.min(video.videoWidth, 640);
    const height = Math.round((video.videoHeight / video.videoWidth) * width);
    this.canvas.width = width;
    this.canvas.height = height;
    const context = this.canvas.getContext("2d");
    if (!context) throw new Error("canvas 2D context is unavailable");
    context.drawImage(video, 0, 0, width, height);

    return new Promise<Blob>((resolve, reject) => {
      this.canvas.toBlob(
        (blob) => blob ? resolve(blob) : reject(new Error("camera frame encoding failed")),
        "image/jpeg",
        0.8,
      );
    });
  }

  private async fetchPrediction(image: Blob): Promise<EmotionApiResponse> {
    const controller = new AbortController();
    const timeout = globalThis.setTimeout(() => controller.abort(), this.options.requestTimeoutMs ?? 10_000);
    try {
      const response = await fetch(`${this.options.url.replace(/\/$/, "")}/v1/emotion`, {
        method: "POST",
        headers: { "Content-Type": "image/jpeg" },
        body: image,
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`emotion API returned ${response.status}`);
      const data = await response.json() as Partial<EmotionApiResponse>;
      if (
        typeof data.emotion !== "string" ||
        typeof data.confidence !== "number" ||
        !isStatus(data.status) ||
        !data.scores ||
        typeof data.scores !== "object"
      ) throw new Error("emotion API returned an invalid response");
      return data as EmotionApiResponse;
    } finally {
      globalThis.clearTimeout(timeout);
    }
  }
}
