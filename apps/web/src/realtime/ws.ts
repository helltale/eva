export type Envelope = {
  version: string;
  type: string;
  requestId: string;
  sessionId: string;
  timestamp: number;
  payload?: unknown;
};

export type RealtimeHandlers = {
  onDelta: (t: string) => void;
  onStatus: (s: string) => void;
  /** Emitted once when assistant text for the turn is final (reload history here). */
  onDone: () => void;
  onTTSChunk?: (seq: number, encoding: string, b64: string) => void;
  onConnection?: (connected: boolean) => void;
  /** After TTS stream ends (barge-in still calls stop from UI). */
  onSpeakingIdle?: () => void;
};

const wsURL = () => {
  const proto = window.location.protocol === "https:" ? "wss" : "ws";
  const host = import.meta.env.DEV ? `${window.location.hostname}:8080` : window.location.host;
  return `${proto}://${host}`;
};

export class Realtime {
  private ws: WebSocket;
  sessionId = "";
  private queue: Array<Envelope & { payload?: unknown }> = [];

  constructor(token: string, private readonly handlers: RealtimeHandlers) {
    const u = `${wsURL()}/ws/v1/realtime?token=${encodeURIComponent(token)}`;
    this.ws = new WebSocket(u);
    this.ws.onopen = () => {
      this.handlers.onConnection?.(true);
      for (const e of this.queue) {
        this.ws.send(JSON.stringify(e));
      }
      this.queue = [];
    };
    this.ws.onclose = () => this.handlers.onConnection?.(false);
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
      if (env.type === "tts.chunk" && env.payload && this.handlers.onTTSChunk) {
        const p =
          typeof env.payload === "string"
            ? (JSON.parse(env.payload) as { sequence?: number; audioEncoding?: string; data?: string })
            : (env.payload as { sequence?: number; audioEncoding?: string; data?: string });
        if (p.data != null && p.sequence != null) {
          this.handlers.onTTSChunk(p.sequence, p.audioEncoding ?? "audio/wav", p.data);
        }
      }
      if (env.type === "assistant.message") {
        this.handlers.onDone();
      }
      if (env.type === "tts.finished") {
        this.handlers.onSpeakingIdle?.();
      }
      if (env.type === "tool.started" || env.type === "tool.finished") {
        this.handlers.onStatus(env.type);
      }
    };
  }

  send(env: Envelope & { payload?: unknown }) {
    if (this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(env));
    } else {
      this.queue.push(env);
    }
  }

  close() {
    this.queue = [];
    this.ws.close();
  }
}

export function connectRealtime(token: string, handlers: RealtimeHandlers): Realtime {
  return new Realtime(token, handlers);
}
