import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateSocket } from "./websocket-client";

class FakeWebSocket extends EventTarget {
  static readonly OPEN = 1;
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
});
