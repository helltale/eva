export type Envelope = {
  version: string;
  type: string;
  requestId: string;
  sessionId: string;
  timestamp: number;
  payload?: unknown;
};

type Handlers = {
  onDelta: (t: string) => void;
  onStatus: (s: string) => void;
  onDone: () => void;
};

const wsURL = () => {
  const proto = window.location.protocol === "https:" ? "wss" : "ws";
  const host = import.meta.env.DEV ? `${window.location.hostname}:8080` : window.location.host;
  return `${proto}://${host}`;
};

export class Realtime {
  private ws: WebSocket;
  private handlers: Handlers;
  sessionId = "";

  constructor(token: string, h: Handlers) {
    this.handlers = h;
    const u = `${wsURL()}/ws/v1/realtime?token=${encodeURIComponent(token)}`;
    this.ws = new WebSocket(u);
    this.ws.onmessage = (ev) => {
      const env = JSON.parse(ev.data as string) as Envelope;
      if (env.sessionId) this.sessionId = env.sessionId;
      if (env.type === "assistant.delta" && env.payload) {
        const p =
          typeof env.payload === "string"
            ? (JSON.parse(env.payload) as { text?: string })
            : (env.payload as { text?: string });
        if (p.text) this.handlers.onDelta(p.text);
      }
      if (env.type === "assistant.message" || env.type === "tts.finished") {
        this.handlers.onDone();
      }
      if (env.type === "tool.started" || env.type === "tool.finished") {
        this.handlers.onStatus(env.type);
      }
    };
  }

  send(env: Envelope & { payload?: unknown }) {
    if (this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(env));
    }
  }

  close() {
    this.ws.close();
  }
}

export function connectRealtime(token: string, handlers: Handlers): Realtime {
  return new Realtime(token, handlers);
}
