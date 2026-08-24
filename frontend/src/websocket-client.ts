import type { Mood } from "./mood";
import type { RecognitionResult } from "./types";

export interface StateSocketOptions {
  url: string;
  token: string;
  roomId: string;
  clientId: string;
  userId?: string;
  reconnectMaxMs?: number;
  statusHeartbeatMs?: number;
}

export class StateSocket extends EventTarget {
  private socket?: WebSocket;
  private closed = false;
  private reconnectAttempt = 0;
  private sequence = 0;
  private lastStatus = "";
  private reconnectTimer?: number;
  private ready = false;
  private lastSentAt = 0;
  private pendingManual?: Mood;

  constructor(private readonly options: StateSocketOptions) {
    super();
  }

  connect(): void {
    this.closed = false;
    this.open();
  }

  sendRecognition(result: RecognitionResult): boolean {
    const now = Date.now();
    const heartbeatMs = this.options.statusHeartbeatMs ?? 60_000;
    if (
      !this.ready ||
      this.socket?.readyState !== WebSocket.OPEN ||
      result.source === "none" ||
      result.status === "unknown" ||
      (result.status === this.lastStatus && now - this.lastSentAt < heartbeatMs)
    ) return false;

    this.lastStatus = result.status;
    this.lastSentAt = now;
    const message = {
      type: "recognition.update",
      sequence: ++this.sequence,
      capturedAt: result.capturedAt,
      hand: result.hand,
      face: result.face ? {
        expression: result.face.expression,
        confidence: result.face.confidence,
      } : null,
      status: result.status,
      source: result.source,
    };
    this.socket.send(JSON.stringify(message));
    this.dispatchEvent(new CustomEvent("recognition.sent", { detail: message }));
    return true;
  }

  // 手動ボタン操作用。sendRecognitionと違い、連続フレーム向けのheartbeat/重複排除は行わず、
  // クリックのたびに必ず1回送信する（単発の明示的な操作のため）。
  sendManual(status: Mood): boolean {
    if (!this.ready || this.socket?.readyState !== WebSocket.OPEN) {
      // The button is intentionally usable while the connection is starting.
      // Keep the latest explicit choice so it is delivered after server.ready.
      this.pendingManual = status;
      return false;
    }

    return this.sendManualNow(status);
  }

  private sendManualNow(status: Mood): boolean {
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) return false;

    this.lastStatus = status;
    this.lastSentAt = Date.now();
    const message = {
      type: "recognition.update",
      sequence: ++this.sequence,
      capturedAt: new Date().toISOString(),
      status,
      source: "manual",
    };
    socket.send(JSON.stringify(message));
    this.dispatchEvent(new CustomEvent("recognition.sent", { detail: message }));
    return true;
  }

  resume(): void {
    if (this.closed) return;
    const readyState = this.socket?.readyState;
    if (readyState === WebSocket.OPEN || readyState === WebSocket.CONNECTING || readyState === WebSocket.CLOSING) return;
    if (this.reconnectTimer !== undefined) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    this.reconnectAttempt = 0;
    this.open();
  }

  close(): void {
    this.closed = true;
    this.ready = false;
    this.pendingManual = undefined;
    this.lastSentAt = 0;
    if (this.reconnectTimer !== undefined) window.clearTimeout(this.reconnectTimer);
    this.socket?.close(1000, "client shutdown");
  }

  private open(): void {
    this.socket = new WebSocket(this.options.url);
    this.socket.addEventListener("open", () => {
      this.reconnectAttempt = 0;
      this.lastStatus = "";
      this.lastSentAt = 0;
      this.socket?.send(JSON.stringify({
        type: "client.hello",
        token: this.options.token,
        roomId: this.options.roomId,
        clientId: this.options.clientId,
        userId: this.options.userId,
      }));
    });
    this.socket.addEventListener("message", (event) => {
      try {
        const data = JSON.parse(String(event.data));
        if (data?.type === "server.ready") {
          this.ready = true;
          const pendingManual = this.pendingManual;
          this.pendingManual = undefined;
          if (pendingManual !== undefined) {
            this.sendManualNow(pendingManual);
          }
          this.dispatchEvent(new Event("open"));
        }
        this.dispatchEvent(new MessageEvent("message", { data }));
      } catch {
        this.dispatchEvent(new CustomEvent("protocol.error", { detail: "server sent invalid JSON" }));
      }
    });
    this.socket.addEventListener("close", () => {
      this.ready = false;
      this.dispatchEvent(new Event("close"));
      if (!this.closed) this.scheduleReconnect();
    });
    this.socket.addEventListener("error", () => this.socket?.close());
  }

  private scheduleReconnect(): void {
    const max = this.options.reconnectMaxMs ?? 30_000;
    const delay = Math.min(1000 * 2 ** this.reconnectAttempt++, max) * (0.8 + Math.random() * 0.4);
    this.reconnectTimer = window.setTimeout(() => this.open(), delay);
  }
}
