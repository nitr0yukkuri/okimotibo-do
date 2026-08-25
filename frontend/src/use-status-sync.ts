import { useEffect, useRef, useState } from "react";
import type { Mood } from "./mood";
import { StateSocket } from "./websocket-client";

export interface StatusSyncOptions {
  url: string;
  token: string;
  roomId: string;
  userId: string;
}

function toMood(value: unknown): Mood | undefined {
  return value === "available" || value === "neutral" || value === "busy" ? value : undefined;
}

// StateSocketはWebSocket用のURL(ws://...)しか持たないため、
// GETでの状態取得に使うHTTP(S)のAPIベースURLをそこから組み立てる。
function statusEndpoint(options: StatusSyncOptions): string {
  const base = options.url.replace(/^ws/, "http").replace(/\/api\/v1\/ws\/?$/, "");
  return `${base}/api/v1/rooms/${encodeURIComponent(options.roomId)}/status/${encodeURIComponent(options.userId)}`;
}

// カメラのON/OFFに関係なく常時接続し、status.changedのブロードキャストを受信して
// 表示用のstatusを更新する。手動ボタンからの送信もこの接続を使う。
export function useStatusSync(options?: StatusSyncOptions): { status: Mood; sendManual: (mood: Mood) => void } {
  const [status, setStatus] = useState<Mood>("neutral");
  const socketRef = useRef<StateSocket | undefined>(undefined);

  useEffect(() => {
    if (!options) return;
    let cancelled = false;
    const clientId = typeof crypto.randomUUID === "function" ? crypto.randomUUID() : Math.random().toString(36).slice(2);
    const socket = new StateSocket({ ...options, clientId });
    socketRef.current = socket;

    let expiryTimer: number | undefined;
    const clearExpiryTimer = () => {
      if (expiryTimer !== undefined) {
        window.clearTimeout(expiryTimer);
        expiryTimer = undefined;
      }
    };
    const scheduleExpiry = (expiresAt: unknown) => {
      clearExpiryTimer();
      if (typeof expiresAt !== "string") return;
      const delay = Date.parse(expiresAt) - Date.now();
      if (!Number.isFinite(delay) || delay > 2_147_000_000) return;
      expiryTimer = window.setTimeout(() => {
        expiryTimer = undefined;
        if (cancelled) return;
        setStatus("neutral");
        void fetchCurrentStatus();
      }, Math.max(0, delay));
    };
    const fetchCurrentStatus = async () => {
      try {
        const headers: HeadersInit = options.token ? { Authorization: `Bearer ${options.token}` } : {};
        const response = await fetch(statusEndpoint(options), { headers });
        if (response.status === 404) {
          clearExpiryTimer();
          if (!cancelled) setStatus("neutral");
          return;
        }
        if (!response.ok || cancelled) return;
        const data = await response.json();
        const mood = toMood(data?.status);
        if (mood && !cancelled) setStatus(mood);
        scheduleExpiry(data?.expiresAt);
      } catch {
        // 初期取得に失敗しても、後続のstatus.changedブロードキャストで復帰できるため無視する。
      }
    };

    // "open"は接続直後だけでなく再接続のたびにも発火するので、
    // 切断中に見逃した変化をここで取りこぼさないようにする。
    socket.addEventListener("open", () => void fetchCurrentStatus());
    socket.addEventListener("message", (event) => {
      const data = (event as MessageEvent).data as {
        type?: string;
        state?: { userId?: string; status?: string; expiresAt?: string };
      };
      if (data?.type !== "status.changed" || data.state?.userId !== options.userId) return;
      const mood = toMood(data.state.status);
      if (mood) setStatus(mood);
      scheduleExpiry(data.state.expiresAt);
    });

    socket.connect();
    const handleVisibilityChange = () => {
      if (document.visibilityState !== "visible") return;
      socket.resume();
      void fetchCurrentStatus();
    };
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      cancelled = true;
      clearExpiryTimer();
      socket.close();
      socketRef.current = undefined;
    };
  }, [options?.url, options?.token, options?.roomId, options?.userId]);

  const sendManual = (mood: Mood) => {
    socketRef.current?.sendManual(mood);
  };

  return { status, sendManual };
}
