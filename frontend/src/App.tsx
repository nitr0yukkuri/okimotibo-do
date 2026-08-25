import { useRef, useState, useEffect, type MouseEvent } from "react";
import type { Session } from "@supabase/supabase-js";
import "./App.css";
import { useStateRecognition } from "./use-state-recognition";
import type { RecognitionResult } from "./types";
import { StatusDisplay } from "./StatusDisplay";
import { StatusPictureInPicture } from "./StatusPictureInPicture";
import { CameraPreviewModal } from "./CameraPreviewModal";
import { ModalFrame } from "./ModalFrame";
import { moodLabel, moodOptions, type Mood } from "./mood";
import { useStatusSync, type StatusSyncOptions } from "./use-status-sync";
import { getAnonymousIdentity, type PairingSession } from "./pairing";
import { supabase } from "./supabase-client";

interface AppProps {
  session?: Session;
  pairing?: PairingSession;
  onLogout: () => void;
}

function resolveWebSocketUrl(configured: string | undefined): string | undefined {
  if (!configured) {
    if (!import.meta.env.DEV) return undefined;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}/api/v1/ws`;
  }
  if (configured.startsWith("ws://")) {
    try {
      const target = new URL(configured);
      const isPrivateHost = target.hostname === "localhost" || target.hostname === "127.0.0.1" ||
        /^(10|192\.168|172\.(1[6-9]|2\d|3[0-1]))\./.test(target.hostname);
      if (isPrivateHost) {
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        return `${protocol}//${window.location.host}/api/v1/ws`;
      }
    } catch {
      return configured;
    }
  }
  return configured;
}

interface RoomSelection {
  roomId?: string;
  loading: boolean;
}

function useRoomId(session: Session | undefined, pairing?: PairingSession): RoomSelection {
  const configuredRoomId = pairing?.roomId ?? import.meta.env.VITE_ROOM_ID;
  const [discoveredRoomId, setDiscoveredRoomId] = useState<string>();
  const [loading, setLoading] = useState(!configuredRoomId && !!session && !!supabase);

  useEffect(() => {
    if (configuredRoomId || !session || !supabase) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    void Promise.resolve(
      supabase
        .from("room_members")
        .select("room_id")
        .eq("user_id", session.user.id)
        .limit(1)
        .then(({ data, error }) => {
          if (cancelled) return;
          if (error) {
            console.warn("Failed to discover a room for the signed-in user.", error);
          }
          setDiscoveredRoomId(data?.[0]?.room_id);
          setLoading(false);
        }),
    ).catch((error: unknown) => {
      if (cancelled) return;
      console.warn("Failed to discover a room for the signed-in user.", error);
      setLoading(false);
    });
    return () => { cancelled = true; };
  }, [configuredRoomId, session?.user.id]);

  return { roomId: configuredRoomId ?? discoveredRoomId, loading };
}

// PC用操作画面
function ControlPanel({ sync, onLogout }: { sync?: StatusSyncOptions; onLogout: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [selectedMood, setSelectedMood] = useState<Mood>("neutral");
  const [cameraStream, setCameraStream] = useState<MediaStream | undefined>();
  const [cameraStreamError, setCameraStreamError] = useState<unknown>();
  const [pipVisible, setPipVisible] = useState(false);
  const [autoRead, setAutoRead] = useState(true);
  const [cameraPreviewOpen, setCameraPreviewOpen] = useState(false);
  const [isHelpOpen, setIsHelpOpen] = useState(false);
  const [clientId] = useState(() => typeof crypto.randomUUID === "function" ? crypto.randomUUID() : Math.random().toString(36).slice(2));
  // カメラ検出専用のソケット(recognition.update送信用)。
  const cameraSocket = sync ? { ...sync, clientId } : undefined;
  // カメラのON/OFFに関わらず常時つながる同期用ソケット。他端末(スマホ等)からの変更もここで受け取る。
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
    enabled: true,
    onStream: (stream, error) => {
      setCameraStream(stream);
      setCameraStreamError(error);
    },
    face: {
      enabled: autoRead,
    },
    socket: cameraSocket,
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
            className="camera-button"
            type="button"
            aria-haspopup="dialog"
            aria-expanded={cameraPreviewOpen}
            onClick={() => setCameraPreviewOpen(true)}
          >
            カメラテスト
          </button>
        </footer>
      </div>
      <video ref={videoRef} hidden muted playsInline />
      {cameraPreviewOpen && (
        <CameraPreviewModal
          mood={selectedMood}
          stream={cameraStream}
          streamError={cameraStreamError}
          onClose={() => setCameraPreviewOpen(false)}
        />
      )}
      {isHelpOpen && (
        <ModalFrame
          backdropClassName="modal-overlay"
          contentClassName="modal-content"
          labelledBy="help-dialog-title"
          onBackdropClick={handleOverlayClick}
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
        </ModalFrame>
      )}
    </main>
  );
}

export function App({ session, pairing, onLogout }: AppProps) {
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
  // ログイン済みならSupabaseセッションの本物のuserId/access_tokenを使い、
  // 未ログイン(ローカル匿名検証)時のみ.envの仮値にフォールバックする。
  const [anonymousIdentity] = useState(() => getAnonymousIdentity());
  const { roomId: discoveredRoomId } = useRoomId(session, pairing);
  const roomId = discoveredRoomId ?? (session || pairing ? undefined : import.meta.env.VITE_ROOM_ID ?? anonymousIdentity.roomId);
  const userId = pairing?.userId ?? session?.user.id ?? import.meta.env.VITE_USER_ID ?? anonymousIdentity.userId;
  const token = pairing?.token ?? session?.access_token ?? "";
  // 開発時だけlocalhostを既定値にし、本番ではRender等のTLS付きURLを必須にする。
  const wsUrl = resolveWebSocketUrl(import.meta.env.VITE_WS_URL);
  const sync: StatusSyncOptions | undefined = roomId && userId && wsUrl
    ? {
        // 127.0.0.1は環境(セキュリティソフト等)によって疎通しないことがあるため、既定値はlocalhostにする。
        url: wsUrl,
        token,
        roomId,
        userId,
      }
    : undefined;

  if (isMobile) {
    return <StatusDisplay sync={sync} onLogout={onLogout} />;
  }

  return <ControlPanel sync={sync} onLogout={onLogout} />;
}
