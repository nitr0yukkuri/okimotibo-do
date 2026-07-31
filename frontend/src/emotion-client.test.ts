import { beforeEach, describe, expect, it, vi } from "vitest";
import { EmotionApiClient } from "./emotion-client";

const response = (status: "available" | "neutral" | "busy" | "unknown", emotion: string, confidence = 0.9) => ({
  ok: true,
  json: async () => ({
    emotion,
    confidence,
    status,
    scores: { Happy: status === "available" ? confidence : 0.05, Anger: status === "busy" ? confidence : 0.05 },
  }),
});

describe("EmotionApiClient", () => {
  beforeEach(() => {
    const context = { drawImage: vi.fn() };
    vi.stubGlobal("document", {
      createElement: () => ({
        width: 0,
        height: 0,
        getContext: () => context,
        toBlob: (callback: (blob: Blob) => void) => callback(new Blob(["image"], { type: "image/jpeg" })),
      }),
    });
    vi.stubGlobal("fetch", vi.fn());
  });

  it("requires three matching predictions before changing status", async () => {
    vi.mocked(fetch).mockResolvedValue(response("available", "Happy") as Response);
    const client = new EmotionApiClient({ url: "http://localhost:8000", inferenceIntervalMs: 1 });
    const video = { readyState: 4, videoWidth: 640, videoHeight: 480 } as HTMLVideoElement;

    expect(await client.recognize(video, 1)).toBeNull();
    expect(await client.recognize(video, 2)).toBeNull();
    expect((await client.recognize(video, 3))?.status).toBe("available");
  });

  it("maps an angry prediction to a busy face result", async () => {
    vi.mocked(fetch).mockResolvedValue(response("busy", "Anger") as Response);
    const client = new EmotionApiClient({
      url: "http://localhost:8000",
      inferenceIntervalMs: 1,
      requiredMatches: 1,
    });
    const video = { readyState: 4, videoWidth: 640, videoHeight: 480 } as HTMLVideoElement;

    const result = await client.recognize(video, 1);
    expect(result?.status).toBe("busy");
    expect(result?.face?.expression).toBe("frown");
  });

  it("clears pending matches when the API returns unknown", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(response("available", "Happy") as Response)
      .mockResolvedValueOnce(response("available", "Happy") as Response)
      .mockResolvedValueOnce(response("unknown", "Neutral", 0.5) as Response)
      .mockResolvedValueOnce(response("available", "Happy") as Response);
    const client = new EmotionApiClient({ url: "http://localhost:8000", inferenceIntervalMs: 1 });
    const video = { readyState: 4, videoWidth: 640, videoHeight: 480 } as HTMLVideoElement;

    expect(await client.recognize(video, 1)).toBeNull();
    expect(await client.recognize(video, 2)).toBeNull();
    expect(await client.recognize(video, 3)).toBeNull();
    expect(await client.recognize(video, 4)).toBeNull();
  });
});
