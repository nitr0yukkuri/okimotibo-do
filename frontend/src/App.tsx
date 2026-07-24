import { useRef, useState } from "react";
import "./App.css";
import { useStateRecognition } from "./use-state-recognition";
import type { RecognitionResult } from "./types";

type Mood = "busy" | "available" | "neutral";

const moodOptions: Array<{ id: Mood; label: string }> = [
  { id: "busy", label: "したくない" },
  { id: "available", label: "したい" },
  { id: "neutral", label: "いいよ" },
];

const moodLabel: Record<Mood, string> = {
  busy: "したくない",
  available: "したい",
  neutral: "いいよ",
};

export function App() {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [selectedMood, setSelectedMood] = useState<Mood>("neutral");
  const [autoRead, setAutoRead] = useState(true);
  const [cameraTesting, setCameraTesting] = useState(false);
  const [clientId] = useState(() => crypto.randomUUID());
  const socket = import.meta.env.VITE_ROOM_ID && import.meta.env.VITE_SUPABASE_ACCESS_TOKEN
    ? {
        url: import.meta.env.VITE_WS_URL ?? "ws://127.0.0.1:8080/api/v1/ws",
        token: import.meta.env.VITE_SUPABASE_ACCESS_TOKEN,
        roomId: import.meta.env.VITE_ROOM_ID,
        clientId,
      }
    : undefined;

  const updateFromHand = (result: RecognitionResult) => {
    if (result.source === "hand" && result.status !== "unknown") {
      setSelectedMood(result.status);
    }
  };

  useStateRecognition(videoRef, {
    enabled: cameraTesting,
    socket,
    onRecognition: updateFromHand,
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
