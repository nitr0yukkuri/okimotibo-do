import { useEffect, useRef, useState } from "react";
import { attachCamera, stopCamera } from "./camera";
import { moodLabel, type Mood } from "./mood";

interface CameraPreviewModalProps {
  mood: Mood;
  onClose: () => void;
}

export function CameraPreviewModal({ mood, onClose }: CameraPreviewModalProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const cameraVideoRef = useRef<HTMLVideoElement>(null);
  const [cameraState, setCameraState] = useState<"starting" | "ready" | "error">("starting");
  const [cameraError, setCameraError] = useState<string | null>(null);

  useEffect(() => {
    let stream: MediaStream | undefined;
    let cancelled = false;

    const startCamera = async () => {
      const video = cameraVideoRef.current;
      if (!video) return;

      try {
        const nextStream = await attachCamera(video);
        if (cancelled) {
          stopCamera(nextStream);
          return;
        }

        stream = nextStream;
        setCameraState("ready");
      } catch (error) {
        if (cancelled) return;

        setCameraState("error");
        setCameraError(
          error instanceof DOMException && error.name === "NotAllowedError"
            ? "カメラの使用が許可されていません。ブラウザの設定からカメラを許可してください。"
            : "カメラを起動できませんでした。カメラが接続されているか確認してください。",
        );
      }
    };

    void startCamera();

    return () => {
      cancelled = true;
      stopCamera(stream);
    };
  }, []);

  useEffect(() => {
    const previousActiveElement = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };

    document.body.style.overflow = "hidden";
    closeButtonRef.current?.focus();
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", handleKeyDown);
      previousActiveElement?.focus();
    };
  }, [onClose]);

  return (
    <div
      className="camera-modal-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        className="camera-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="camera-modal-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="camera-modal-header">
          <div>
            <p className="camera-modal-kicker">表示プレビュー</p>
            <h2 id="camera-modal-title">カメラ表示テスト</h2>
          </div>
          <button className="camera-modal-icon-button" type="button" aria-label="閉じる" onClick={onClose}>
            ×
          </button>
        </div>

        <div className={`camera-modal-preview preview-${mood}`}>
          <div className="camera-preview-frame">
            <video
              ref={cameraVideoRef}
              className="camera-preview-video"
              autoPlay
              muted
              playsInline
              aria-label="カメラ映像"
            />
            {cameraState !== "ready" && (
              <div className={`camera-preview-message camera-preview-message-${cameraState}`} role={cameraState === "error" ? "alert" : undefined}>
                {cameraState === "starting" ? "カメラを起動しています…" : cameraError}
              </div>
            )}
            <span className="camera-preview-corner camera-preview-corner-top-left" />
            <span className="camera-preview-corner camera-preview-corner-top-right" />
            <span className="camera-preview-corner camera-preview-corner-bottom-left" />
            <span className="camera-preview-corner camera-preview-corner-bottom-right" />
          </div>
          <div className="camera-preview-status">
            <strong>{moodLabel[mood]}</strong>
            <span>現在の状態を表示しています</span>
          </div>
        </div>

        <p className="camera-modal-note">実際のカメラ映像を表示しています。閉じるとカメラを停止します。</p>
        <button className="camera-modal-close" type="button" ref={closeButtonRef} onClick={onClose}>
          閉じる
        </button>
      </section>
    </div>
  );
}
