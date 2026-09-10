export type Gesture = "thumb_up" | "sideways_thumb" | "thumb_down" | "unknown";
export type Status = "available" | "neutral" | "busy" | "unknown";
export type Expression = "smile" | "frown" | "surprised" | "neutral" | "unknown";
export type RecognitionSource = "hand" | "face" | "none";

export interface Landmark {
  x: number;
  y: number;
  z: number;
}

export interface RecognitionResult {
  capturedAt: string;
  hand: {
    gesture: Gesture;
    confidence: number;
    handedness?: "Left" | "Right";
  } | null;
  face: {
    expression: Expression;
    confidence: number;
    blendshapes: Record<string, number>;
    calibrated: boolean;
    calibrationProgress: number;
    scores: {
      available: number;
      busy: number;
      surprised: number;
      browTension: number;
      eyeTension: number;
      mouthTension: number;
    };
  } | null;
  status: Status;
  source: RecognitionSource;
}

export const GESTURE_STATUS: Record<Gesture, Status> = {
  thumb_up: "available",
  sideways_thumb: "neutral",
  thumb_down: "busy",
  unknown: "unknown",
};

// Facial movement is only a fallback. An explicit hand gesture always wins.
export const EXPRESSION_STATUS: Record<Expression, Status> = {
  smile: "available",
  frown: "busy",
  surprised: "unknown",
  neutral: "unknown",
  unknown: "unknown",
};
