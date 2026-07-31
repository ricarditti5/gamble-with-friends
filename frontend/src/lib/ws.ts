import type { ServerMsg } from "../types";

export type ConnStatus = "connecting" | "open" | "reconnecting" | "closed";

// WebSocket client with exponential backoff reconnect (RF4.3). The session id
// is sent on join, so a reload resumes the same seat.
export class GameClient {
  onMessage: (msg: ServerMsg) => void = () => {};
  onStatus: (status: ConnStatus) => void = () => {};

  private ws: WebSocket | null = null;
  private sessionId: string;
  private nickname: string;
  private roomCode: string;
  private attempts = 0;
  private closedByUser = false;
  private reconnectTimer: number | null = null;

  constructor(sessionId: string, nickname: string, roomCode: string) {
    this.sessionId = sessionId;
    this.nickname = nickname;
    this.roomCode = roomCode;
  }

  connect() {
    this.closedByUser = false;
    this.open();
  }

  private wsUrl(): string {
    const base = import.meta.env.VITE_WS_URL ?? "";
    if (base) return `${base}/ws?room=${encodeURIComponent(this.roomCode)}`;
    const proto = location.protocol === "https:" ? "wss" : "ws";
    return `${proto}://${location.host}/ws?room=${encodeURIComponent(this.roomCode)}`;
  }

  private open() {
    this.onStatus(this.attempts === 0 ? "connecting" : "reconnecting");
    const ws = new WebSocket(this.wsUrl());
    this.ws = ws;

    ws.onopen = () => {
      this.attempts = 0;
      this.onStatus("open");
      ws.send(JSON.stringify({ type: "join", session_id: this.sessionId, nickname: this.nickname }));
    };

    ws.onmessage = (ev) => {
      try {
        this.onMessage(JSON.parse(ev.data) as ServerMsg);
      } catch {
        // ignore malformed frames
      }
    };

    ws.onclose = () => {
      if (this.closedByUser) return;
      this.scheduleReconnect();
    };

    ws.onerror = () => {
      ws.close();
    };
  }

  private scheduleReconnect() {
    if (this.reconnectTimer !== null) return;
    const delay = Math.min(1000 * 2 ** this.attempts, 10000);
    this.attempts++;
    this.onStatus("reconnecting");
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.open();
    }, delay);
  }

  send(msg: Record<string, unknown>) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  close() {
    this.closedByUser = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.ws?.close();
    this.onStatus("closed");
  }
}
