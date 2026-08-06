import { useEffect, useRef } from "react";
import { attachCamera, stopCamera } from "./camera";
import { EmotionApiClient, type EmotionApiOptions } from "./emotion-client";
import { MediaPipeFaceRecognizer, type MediaPipeFaceOptions } from "./mediapipe-face-recognizer";
import { MediaPipeStateRecognizer, type RecognizerOptions } from "./recognizer";
import type { RecognitionResult } from "./types";
import { StateSocket, type StateSocketOptions } from "./websocket-client";

export interface UseStateRecognitionOptions {
  enabled?: boolean;
  onStream?: (stream: MediaStream | undefined, error?: unknown) => void;
  recognizer?: RecognizerOptions;
  face?: MediaPipeFaceOptions;
  emotion?: EmotionApiOptions;
  socket?: StateSocketOptions;
  onRecognition?: (result: RecognitionResult) => void;
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
    const face = optionsRef.current.face ? new MediaPipeFaceRecognizer(optionsRef.current.face) : undefined;
    const emotion = optionsRef.current.emotion ? new EmotionApiClient(optionsRef.current.emotion) : undefined;
    const socket = optionsRef.current.socket ? new StateSocket(optionsRef.current.socket) : undefined;
    let stream: MediaStream | undefined;
    let animationFrame = 0;
    let cancelled = false;
    let lastHandAt = 0;
    let emotionErrorReported = false;

    const start = async () => {
      const cameraPromise = attachCamera(video).then((cameraStream) => {
        stream = cameraStream;
        if (cancelled) {
          stopCamera(cameraStream);
          return cameraStream;
        }
        optionsRef.current.onStream?.(cameraStream);
        return cameraStream;
      });
      try {
        await Promise.all([
          cameraPromise,
          recognizer.initialize(),
          face?.initialize(),
        ]);
      } catch (error) {
        cancelled = true;
        optionsRef.current.onStream?.(undefined, error);
        stopCamera(stream);
        recognizer.close();
        face?.close();
        throw error;
      }
      if (cancelled) {
        stopCamera(stream);
        recognizer.close();
        face?.close();
        return;
      }
      socket?.connect();
      const loop = (timestamp: number) => {
        const result = recognizer.recognize(video, timestamp);
        if (result) {
          lastHandAt = Date.now();
          optionsRef.current.onRecognition?.(result);
          socket?.sendRecognition(result);
        }
        if (face && optionsRef.current.face?.enabled !== false) {
          const faceResult = face.recognize(video, timestamp);
          const handPriorityMs = 3_000;
          if (faceResult && Date.now() - lastHandAt >= handPriorityMs) {
            optionsRef.current.onRecognition?.(faceResult);
            socket?.sendRecognition(faceResult);
          }
        }
        if (emotion && optionsRef.current.emotion?.enabled !== false) {
          void emotion.recognize(video, timestamp).then((faceResult) => {
            emotionErrorReported = false;
            if (!faceResult || cancelled || optionsRef.current.emotion?.enabled === false) return;
            const handPriorityMs = 3_000;
            if (Date.now() - lastHandAt < handPriorityMs) return;
            optionsRef.current.onRecognition?.(faceResult);
            socket?.sendRecognition(faceResult);
          }).catch((error) => {
            if (emotionErrorReported || cancelled) return;
            emotionErrorReported = true;
            window.dispatchEvent(new CustomEvent("emotion.error", { detail: error }));
          });
        }
        animationFrame = requestAnimationFrame(loop);
      };
      animationFrame = requestAnimationFrame(loop);
    };
    void start().catch((error) => window.dispatchEvent(new CustomEvent("recognition.error", { detail: error })));

    return () => {
      cancelled = true;
      optionsRef.current.onStream?.(undefined);
      cancelAnimationFrame(animationFrame);
      socket?.close();
      recognizer.close();
      face?.close();
      emotion?.reset();
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
    options.face?.delegate,
    options.face?.faceModelUrl,
    options.face?.inferenceIntervalMs,
    options.face?.lockMs,
    options.face?.minConfidence,
    options.face?.smileHoldMs,
    options.face?.wasmRoot,
    options.emotion?.inferenceIntervalMs,
    options.emotion?.requestTimeoutMs,
    options.emotion?.requiredMatches,
    options.emotion?.url,
    options.emotion?.windowSize,
    options.socket?.clientId,
    options.socket?.reconnectMaxMs,
    options.socket?.roomId,
    options.socket?.token,
    options.socket?.url,
    options.socket?.userId,
  ]);
}
