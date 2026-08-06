import { useRef, useState, useEffect, type MouseEvent } from "react";
import "./App.css";
import { useStateRecognition } from "./use-state-recognition";
import type { RecognitionResult } from "./types";
import { StatusDisplay } from "./StatusDisplay";
import { StatusPictureInPicture } from "./StatusPictureInPicture";
import { CameraPreviewModal } from "./CameraPreviewModal";
import { moodLabel, moodOptions, type Mood } from "./mood";

interface AppProps {
  onLogout: () => void;
}

// PC用操作画面
function ControlPanel({ onLogout }: AppProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [selectedMood, setSelectedMood] = useState<Mood>("neutral");
  const [pipVisible, setPipVisible] = useState(false);
  const [autoRead, setAutoRead] = useState(true);
  const [cameraTesting, setCameraTesting] = useState(true);
  const [cameraPreviewOpen, setCameraPreviewOpen] = useState(false);
  const [isHelpOpen, setIsHelpOpen] = useState(false);
  const [clientId] = useState(() => typeof crypto.randomUUID === "function" ? crypto.randomUUID() : Math.random().toString(36).slice(2));
  const socket = import.meta.env.VITE_ROOM_ID && import.meta.env.VITE_SUPABASE_ACCESS_TOKEN
    ? {
        url: import.meta.env.VITE_WS_URL ?? "ws://127.0.0.1:8080/api/v1/ws",
        token: import.meta.env.VITE_SUPABASE_ACCESS_TOKEN,
        roomId: import.meta.env.VITE_ROOM_ID,
        clientId,
      }
    : undefined;

  const updateFromRecognition = (result: RecognitionResult) => {
    if (result.status !== "unknown") {
      setSelectedMood(result.status);
    }
  };

  useStateRecognition(videoRef, {
    enabled: cameraTesting,
    face: {
      enabled: autoRead,
    },
    socket,
    onRecognition: updateFromRecognition,
  });

  useEffect(() => {
    if (!isHelpOpen) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsHelpOpen(false);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isHelpOpen]);

  const handleOverlayClick = (event: MouseEvent<HTMLDivElement>) => {
    if (event.target === event.currentTarget) {
      setIsHelpOpen(false);
    }
  };

  return (
    <main className="board" aria-label="おきもちぼ〜ど">
      <header className="board-header">
        <button
          className="help-button"
          aria-label="ヘルプ"
          type="button"
          onClick={() => setIsHelpOpen(true)}
        >
          ?
        </button>
        <img className="brand-logo" src="/okimochi_logo.png" alt="おきもちぼ〜ど" />
        <button className="logout-button" type="button" onClick={onLogout}>
          ログアウト
        </button>
      </header>

      <div className="board-content">
        <section className="talk-panel" aria-labelledby="talk-heading">
          <h2 id="talk-heading">お話</h2>

          <div className="mood-controls" role="group" aria-label="会話したい気持ち">
            {moodOptions.map((option) => (
              <button
                className={`mood-button mood-${option.id}`}
                type="button"
                aria-pressed={selectedMood === option.id}
                key={option.id}
                onClick={() => setSelectedMood(option.id)}
              >
                {option.label}
              </button>
            ))}
          </div>

          <div className="room-stage" aria-label={`現在の気持ち: ${moodLabel[selectedMood]}`}>
            <div className={`status-zone status-${selectedMood}`} />
          </div>

        </section>

        <footer className="board-footer">
          <div className="auto-read">
            <StatusPictureInPicture
              enabled={pipVisible}
              mood={selectedMood}
              onEnabledChange={setPipVisible}
              onMoodChange={setSelectedMood}
            />
            <label className="toggle-row">
              <span>表情読み取りでステータス更新</span>
              <input
                type="checkbox"
                checked={autoRead}
                onChange={(event) => setAutoRead(event.target.checked)}
              />
            </label>
            <p>※この機能はベータ版であり、</p>
            <p>本来の意図と異なる動作をする可能性があります。</p>
          </div>

          <button
            className="camera-button"
            type="button"
            aria-haspopup="dialog"
            aria-expanded={cameraPreviewOpen}
            onClick={() => {
              setCameraTesting((current) => !current);
              setCameraPreviewOpen(true);
            }}
          >
            カメラテスト
          </button>
        </footer>
      </div>
      <video ref={videoRef} hidden muted playsInline />
      {cameraPreviewOpen && (
        <CameraPreviewModal mood={selectedMood} onClose={() => setCameraPreviewOpen(false)} />
      )}
      {isHelpOpen && (
        <div className="modal-overlay" onClick={handleOverlayClick}>
          <div
            className="modal-content"
            role="dialog"
            aria-modal="true"
            aria-labelledby="help-dialog-title"
          >
            <button
              className="modal-close-button"
              type="button"
              aria-label="ヘルプを閉じる"
              onClick={() => setIsHelpOpen(false)}
            >
              ×
            </button>
            <h3 id="help-dialog-title">おきもちぼーど<br />操作方法</h3>
            <p className="modal-description">
              パソコンでの作業中に邪魔されたくない時や、逆に話しかけて欲しい時に簡単に意思表示できるシステムです！
            </p>

            <div className="modal-guide-container">
              <h4 className="modal-guide-title">使い方</h4>
              <ol className="modal-guide-list">
                <li>PCとスマホで同じアカウントにログインする</li>
                <li>スマホを周囲から見える位置に置く</li>
                <li>PCカメラにハンドサインをすると、スマホの表示が変わります！</li>
              </ol>
              <p className="modal-guide-note">
                ※正しく動作されない場合、PC画面からの直接操作も可能です
              </p>

              <h4 className="modal-status-title">ステータス一覧</h4>
              <ul className="modal-status-list">
                <li><span className="status-red">・作業中</span> 集中したい時 👎</li>
                <li><span className="status-yellow">・対応可能</span> 用事があれば応えられる時 横グッド</li>
                <li><span className="status-green">・暇</span> おしゃべりしたい時 👍</li>
              </ul>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

export function App({ onLogout }: AppProps) {
  const [isMobile, setIsMobile] = useState(() => {
    const userAgent = window.navigator.userAgent;
    const mobileRegex = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i;
    return mobileRegex.test(userAgent) || window.innerWidth <= 768;
  });

  useEffect(() => {
    const handleResize = () => setIsMobile(window.innerWidth <= 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(window.navigator.userAgent));
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  if (isMobile) {
    return <StatusDisplay mood="available" onLogout={onLogout} />;
  }

  return <ControlPanel onLogout={onLogout} />;
}
