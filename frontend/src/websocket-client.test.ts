import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateSocket } from "./websocket-client";

class FakeWebSocket extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  readonly sent: string[] = [];
  readyState = 0;

  constructor(readonly url: string) {
    super();
  }

  send(message: string): void {
    this.sent.push(message);
  }

  close(): void {
    this.readyState = 3;
  }

  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.dispatchEvent(new Event("open"));
  }

  announceReady(): void {
    this.dispatchEvent(new MessageEvent("message", {
      data: JSON.stringify({ type: "server.ready" }),
    }));
  }
}

describe("StateSocket", () => {
  beforeEach(() => {
    vi.stubGlobal("WebSocket", FakeWebSocket);
    vi.stubGlobal("window", {
      clearTimeout: vi.fn(),
      setTimeout: vi.fn(),
    });
  });

  it("delivers the latest manual choice after server.ready", () => {
    const socket = new StateSocket({
      url: "ws://localhost:8080/api/v1/ws",
      token: "",
      roomId: "room",
      clientId: "client",
      userId: "user",
    });

    expect(socket.sendManual("busy")).toBe(false);
    socket.connect();
    const connection = (socket as unknown as { socket: FakeWebSocket }).socket;
    connection.open();
    connection.announceReady();

    expect(JSON.parse(connection.sent[1])).toMatchObject({
      type: "recognition.update",
      status: "busy",
      source: "manual",
    });
  });

  it("can send the current hand state again after a manual broadcast", () => {
    const socket = new StateSocket({
      url: "ws://localhost:8080/api/v1/ws",
      token: "",
      roomId: "room",
      clientId: "client",
      userId: "user",
    });
    socket.connect();
    const connection = (socket as unknown as { socket: FakeWebSocket }).socket;
    connection.open();
    connection.announceReady();

    const result = {
      capturedAt: new Date().toISOString(),
      hand: { gesture: "thumb_up" as const, confidence: .9 },
      face: null,
      status: "available" as const,
      source: "hand" as const,
    };
    expect(socket.sendRecognition(result)).toBe(true);
    expect(socket.sendRecognition(result)).toBe(false);
    socket.resetRecognitionDeduplication();
    expect(socket.sendRecognition(result)).toBe(true);
  });

  it("does not create duplicate connections when connect is called twice", () => {
    const socket = new StateSocket({
      url: "ws://localhost:8080/api/v1/ws",
      token: "",
      roomId: "room",
      clientId: "client",
      userId: "user",
    });

    socket.connect();
    const firstConnection = (socket as unknown as { socket: FakeWebSocket }).socket;
    socket.connect();

    expect((socket as unknown as { socket: FakeWebSocket }).socket).toBe(firstConnection);
  });

  it("schedules only one reconnect after repeated close notifications", () => {
    const reconnectCallbacks: Array<() => void> = [];
    const setTimeout = vi.fn((callback: () => void) => {
      reconnectCallbacks.push(callback);
      return reconnectCallbacks.length;
    });
    vi.stubGlobal("window", { clearTimeout: vi.fn(), setTimeout });

    const socket = new StateSocket({
      url: "ws://localhost:8080/api/v1/ws",
      token: "",
      roomId: "room",
      clientId: "client",
      userId: "user",
    });
    socket.connect();
    const connection = (socket as unknown as { socket: FakeWebSocket }).socket;
    connection.open();
    connection.announceReady();

    connection.dispatchEvent(new Event("close"));
    connection.dispatchEvent(new Event("close"));

    expect(setTimeout).toHaveBeenCalledTimes(1);
    expect(reconnectCallbacks).toHaveLength(1);
  });
});
