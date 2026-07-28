import { useEffect, useRef } from "react";
import { attachCamera, stopCamera } from "./camera";
import { MediaPipeStateRecognizer, type RecognizerOptions } from "./recognizer";
import type { RecognitionResult } from "./types";
import { StateSocket, type StateSocketOptions } from "./websocket-client";

export interface UseStateRecognitionOptions {
  enabled?: boolean;
  recognizer?: RecognizerOptions;
  socket?: StateSocketOptions;
  onRecognition?: (result: RecognitionResult) => void;
  onError?: (error: unknown) => void;
}

export function useStateRecognition(
  videoRef: React.RefObject<HTMLVideoElement | null>,
  options: UseStateRecognitionOptions,
): void {
  const optionsRef = useRef(options);
  optionsRef.current = options;

  useEffect(() => {
    if (optionsRef.current.enabled === false || !videoRef.current) return;
    const video = videoRef.current;
    const recognizer = new MediaPipeStateRecognizer(optionsRef.current.recognizer);
    const socket = optionsRef.current.socket ? new StateSocket(optionsRef.current.socket) : undefined;
    let stream: MediaStream | undefined;
    let animationFrame = 0;
    let cancelled = false;

    const start = async () => {
      stream = await attachCamera(video);
      await recognizer.initialize();
      if (cancelled) {
        stopCamera(stream);
        recognizer.close();
        return;
      }
      socket?.connect();
      const loop = (timestamp: number) => {
        const result = recognizer.recognize(video, timestamp);
        if (result) {
          optionsRef.current.onRecognition?.(result);
          socket?.sendRecognition(result);
        }
        animationFrame = requestAnimationFrame(loop);
      };
      animationFrame = requestAnimationFrame(loop);
    };
    void start().catch((error) => {
      optionsRef.current.onError?.(error);
      window.dispatchEvent(new CustomEvent("recognition.error", { detail: error }));
    });

    return () => {
      cancelled = true;
      cancelAnimationFrame(animationFrame);
      socket?.close();
      recognizer.close();
      stopCamera(stream);
    };
  }, [
    videoRef,
    options.enabled,
    options.recognizer?.delegate,
    options.recognizer?.gestureModelUrl,
    options.recognizer?.inferenceIntervalMs,
    options.recognizer?.minConfidence,
    options.recognizer?.wasmRoot,
    options.socket?.clientId,
    options.socket?.reconnectMaxMs,
    options.socket?.roomId,
    options.socket?.token,
    options.socket?.url,
    options.socket?.userId,
  ]);
}
