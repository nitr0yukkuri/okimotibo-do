import { useRef, useState, useEffect } from "react";
import "./App.css";
import { useStateRecognition } from "./use-state-recognition";
import type { RecognitionResult } from "./types";
import { StatusDisplay } from "./StatusDisplay";
import { StatusPictureInPicture } from "./StatusPictureInPicture";
import { moodLabel, moodOptions, type Mood } from "./mood";
import { useStatusSync, type StatusSyncOptions } from "./use-status-sync";

// PC用操作画面
function ControlPanel({ sync }: { sync?: StatusSyncOptions }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [selectedMood, setSelectedMood] = useState<Mood>("neutral");
  const [pipVisible, setPipVisible] = useState(false);
  const [autoRead, setAutoRead] = useState(true);
  const [cameraTesting, setCameraTesting] = useState(true);
  const [clientId] = useState(() => typeof crypto.randomUUID === "function" ? crypto.randomUUID() : Math.random().toString(36).slice(2));
  // カメラ検出専用のソケット。cameraTestingがfalseの間は接続しない。
  const cameraSocket = sync ? { ...sync, clientId } : undefined;
  // カメラのON/OFFに関わらず常時つながる同期用ソケット。他端末からの変更もここで受け取る。
  const { status: syncedMood, sendManual } = useStatusSync(sync);

  useEffect(() => {
    setSelectedMood(syncedMood);
  }, [syncedMood]);

  const updateFromRecognition = (result: RecognitionResult) => {
    if (result.status !== "unknown") {
      setSelectedMood(result.status);
    }
  };

  // ボタン操作は画面表示をすぐ切り替えつつ、サーバーにも送って他端末へ配信する。
  const handleManualMoodChange = (mood: Mood) => {
    setSelectedMood(mood);
    sendManual(mood);
  };

  useStateRecognition(videoRef, {
    enabled: cameraTesting,
    face: {
      enabled: autoRead,
    },
    socket: cameraSocket,
    onRecognition: updateFromRecognition,
  });

  return (
    <main className="board" aria-label="おきもちぼ〜ど">
      <header className="board-header">
        <button className="help-button" aria-label="ヘルプ">
          ?
        </button>
        <img className="brand-logo" src="/okimochi_logo.png" alt="おきもちぼ〜ど" />
        <button className="logout-button" type="button">
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
                onClick={() => handleManualMoodChange(option.id)}
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
              onMoodChange={handleManualMoodChange}
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
            className={`camera-button${cameraTesting ? " is-testing" : ""}`}
            type="button"
            onClick={() => setCameraTesting((current) => !current)}
          >
            {cameraTesting ? "カメラ停止" : "カメラテスト"}
          </button>
        </footer>
      </div>
      <video ref={videoRef} hidden muted playsInline />
    </main>
  );
}

export function App() {
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

  // PC・スマホの両画面で同じルーム/ユーザーの状態を同期させるための接続設定。
  // 本来はGoogleログイン後のSupabaseセッションから取るべき値だが、
  // ログイン機能が未実装の現段階ではVITE_ROOM_ID/VITE_USER_IDで代用する（ローカル匿名モード用）。
  const roomId = import.meta.env.VITE_ROOM_ID;
  const userId = import.meta.env.VITE_USER_ID;
  const token = import.meta.env.VITE_SUPABASE_ACCESS_TOKEN ?? "";
  const sync: StatusSyncOptions | undefined = roomId && userId
    ? {
        url: import.meta.env.VITE_WS_URL ?? "ws://127.0.0.1:8080/api/v1/ws",
        token,
        roomId,
        userId,
      }
    : undefined;

  if (isMobile) {
    return <StatusDisplay sync={sync} />;
  }

  return <ControlPanel sync={sync} />;
}
