import { useRef, useState, useEffect } from "react";
import "./App.css";
import { useStateRecognition } from "./use-state-recognition";
import type { RecognitionResult } from "./types";
import { StatusDisplay } from "./StatusDisplay";

type Mood = "busy" | "available" | "neutral";

const moodOptions: Array<{ id: Mood; label: string }> = [
  { id: "available", label: "したい" },
  { id: "neutral", label: "いいよ" },
  { id: "busy", label: "したくない" },
];

const moodLabel: Record<Mood, string> = {
  available: "したい",
  neutral: "いいよ",
  busy: "したくない",
};

// PC用操作画面
function ControlPanel() {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [selectedMood, setSelectedMood] = useState<Mood>("neutral");
  const [autoRead, setAutoRead] = useState(true);
  const [cameraTesting, setCameraTesting] = useState(false);
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

  //モーダルの背景をクリックした時に閉じる処理
  const handleOverlayClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) {
      setIsHelpOpen(false);
    }
  };

  return (
    <main className="board" aria-label="おきもちぼ〜ど">
      <header className="board-header">
        <button 
          className="help-button" 
          aria-label="ヘルプ"
          onClick={() => setIsHelpOpen(true)}
        >
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
      {isHelpOpen && (
        <div className="modal-overlay" onClick={handleOverlayClick}>
          <div className="modal-content">
            <h3>おきもちぼーど<br />操作方法</h3>
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
                <li><span className="status-red">・作業中</span>  集中したい時    👎</li>
                <li><span className="status-yellow">・対応可能</span> 用事があれば応えられる時 ハンドサイン</li>
                <li><span className="status-green">・暇</span>    おしゃべりしたい時 👍</li>
              </ul>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

// ルーティング用コンポーネント
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

  // スマホからのアクセスの場合は全画面表示コンポーネントへ
  if (isMobile) {
    return <StatusDisplay mood="available" />;
  }

  // PCからのアクセスの場合は操作画面へ
  return <ControlPanel />;
}
