import type { Gesture, Landmark } from "./types";

const WRIST = 0;
const THUMB_CMC = 1;
const THUMB_MCP = 2;
const THUMB_IP = 3;
const THUMB_TIP = 4;
const INDEX_MCP = 5;
const INDEX_PIP = 6;
const INDEX_TIP = 8;
const MIDDLE_MCP = 9;
const MIDDLE_PIP = 10;
const MIDDLE_TIP = 12;
const RING_MCP = 13;
const RING_PIP = 14;
const RING_TIP = 16;
const PINKY_MCP = 17;
const PINKY_PIP = 18;
const PINKY_TIP = 20;

export interface GestureClassification {
  gesture: Gesture;
  confidence: number;
  isFist?: boolean;
  isUnsupported?: boolean;
}

function distance(a: Landmark, b: Landmark): number {
  return Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z);
}

function angle(a: Landmark, b: Landmark, c: Landmark): number {
  const ab = { x: a.x - b.x, y: a.y - b.y, z: a.z - b.z };
  const cb = { x: c.x - b.x, y: c.y - b.y, z: c.z - b.z };
  const dot = ab.x * cb.x + ab.y * cb.y + ab.z * cb.z;
  const magnitude = Math.hypot(ab.x, ab.y, ab.z) * Math.hypot(cb.x, cb.y, cb.z);
  return Math.acos(Math.max(-1, Math.min(1, dot / Math.max(magnitude, 1e-9))));
}

// 閾値を2.3に統一してグレーゾーンをなくす。
function isFolded(points: Landmark[], mcp: number, pip: number, tip: number): boolean {
  return (
    angle(points[mcp], points[pip], points[tip]) <= 2.3 ||
    distance(points[tip], points[WRIST]) < distance(points[pip], points[WRIST]) * 1.08
  );
}

// 親指の距離閾値を 0.62→0.68 に強化（グーでも親指が出ていると誤判定されにくくする）
function thumbIsExtended(points: Landmark[], scale: number): boolean {
  const straight = angle(points[THUMB_MCP], points[THUMB_IP], points[THUMB_TIP]) > 2.25;
  const awayFromPalm = distance(points[THUMB_TIP], points[INDEX_MCP]) > scale * 0.68;
  return straight && awayFromPalm;
}

export function classifyLandmarks(points: Landmark[]): GestureClassification {
  if (points.length !== 21) return { gesture: "unknown", confidence: 0 };

  const scale = distance(points[WRIST], points[MIDDLE_MCP]);
  if (scale < 1e-5) return { gesture: "unknown", confidence: 0 };

  const thumb = thumbIsExtended(points, scale);
  const indexFolded = isFolded(points, INDEX_MCP, INDEX_PIP, INDEX_TIP);
  const middleFolded = isFolded(points, MIDDLE_MCP, MIDDLE_PIP, MIDDLE_TIP);
  const ringFolded = isFolded(points, RING_MCP, RING_PIP, RING_TIP);
  const pinkyFolded = isFolded(points, PINKY_MCP, PINKY_PIP, PINKY_TIP);

  // 🤙は対応ジェスチャーに含めない。MediaPipeがthumb_upと誤認しても
  // この入力を状態更新へ流さないため、明示的に無視する。
  const pinkyExtended = !pinkyFolded;
  if (thumb && indexFolded && middleFolded && ringFolded && pinkyExtended) {
    return { gesture: "unknown", confidence: 0, isUnsupported: true };
  }

  if (indexFolded && middleFolded && ringFolded && pinkyFolded && thumbIsNearPalm(points, scale)) {
    return { gesture: "unknown", confidence: 0, isFist: true };
  }

  if (!(thumb && indexFolded && middleFolded && ringFolded && pinkyFolded)) {
    return { gesture: "unknown", confidence: 0 };
  }

  const thumbVectorY = points[THUMB_TIP].y - points[THUMB_CMC].y;
  const verticality = Math.abs(thumbVectorY) / Math.max(distance(points[THUMB_TIP], points[THUMB_CMC]), 1e-9);
  if (verticality < 0.55) {
    return { gesture: "sideways_thumb", confidence: 0.86 };
  }

  const confidence = Math.min(0.95, 0.72 + verticality * 0.2);
  return thumbVectorY < 0
    ? { gesture: "thumb_up", confidence }
    : { gesture: "thumb_down", confidence };
}

export function normalizeMediaPipeGesture(name?: string): Gesture {
  if (name === "Thumb_Up") return "thumb_up";
  if (name === "Thumb_Down") return "thumb_down";
  return "unknown";
}

function thumbIsNearPalm(points: Landmark[], scale: number): boolean {
  const distanceToPalm = Math.min(
    distance(points[THUMB_TIP], points[INDEX_MCP]),
    distance(points[THUMB_TIP], points[MIDDLE_MCP]),
  );
  return distanceToPalm <= scale * 0.65;
}
