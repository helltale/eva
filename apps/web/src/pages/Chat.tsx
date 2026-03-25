import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import * as api from "../api/client";
import type { Message } from "../api/client";
import { startMicStreaming } from "../audio/mic";
import { PlaybackManager } from "../audio/playback";
import { useAuth } from "../auth";
import { connectRealtime, Realtime } from "../realtime/ws";

type VoicePhase = "idle" | "listening" | "speaking" | "thinking";

export function ChatPage() {
  const { state, logout } = useAuth();
  const token = state.access!;
  const [conversations, setConversations] = useState<api.Conversation[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState("");
  const [status, setStatus] = useState("");
  const [toolStatus, setToolStatus] = useState("");
  const [wsConnected, setWsConnected] = useState(false);
  const [voicePhase, setVoicePhase] = useState<VoicePhase>("idle");
  const [micOn, setMicOn] = useState(false);
  const rtRef = useRef<Realtime | null>(null);
  const playbackRef = useRef(new PlaybackManager());
  const stopMicRef = useRef<(() => void) | null>(null);

  const loadConvos = useCallback(async () => {
    const list = await api.listConversations(token);
    setConversations(list);
    if (!activeId && list.length > 0) setActiveId(list[0].id);
  }, [token, activeId]);

  useEffect(() => {
    loadConvos().catch(() => setStatus("failed to list conversations"));
  }, [loadConvos]);

  useEffect(() => {
    if (!activeId) return;
    setStatus("loading messages…");
    api
      .listMessages(token, activeId)
      .then(setMessages)
      .catch(() => setStatus("messages error"))
      .finally(() => setStatus(""));
  }, [token, activeId]);

  useEffect(() => {
    const pb = playbackRef.current;
    return () => {
      stopMicRef.current?.();
      pb.dispose();
      rtRef.current?.close();
    };
  }, []);

  async function newChat() {
    const c = await api.createConversation(token, "New chat");
    setConversations((x) => [c, ...x]);
    setActiveId(c.id);
    setMessages([]);
  }

  async function onSend(e: FormEvent) {
    e.preventDefault();
    if (!activeId || !input.trim()) return;
    const text = input.trim();
    setInput("");
    setStreaming("");
    setToolStatus("");
    const userMsg: Message = {
      id: crypto.randomUUID(),
      conversationId: activeId,
      role: "user",
      content: text,
      createdAt: new Date().toISOString(),
    };
    setMessages((m) => [...m, userMsg]);
    setStatus("thinking…");
    setVoicePhase("thinking");

    try {
      let acc = "";
      await api.chatMessageStream(
        token,
        { conversationId: activeId, content: text, stream: true },
        (d) => {
          acc += d;
          setStreaming(acc);
        },
        async (final) => {
          setStreaming("");
          setStatus("");
          setVoicePhase("idle");
          const msgs = await api.listMessages(token, activeId);
          setMessages(msgs);
          void final;
        }
      );
    } catch {
      setStatus("chat error");
      setVoicePhase("idle");
    }
  }

  function openRealtime() {
    if (!activeId || !token) return;
    playbackRef.current.stop();
    rtRef.current?.close();
    setVoicePhase("idle");
    setStreaming("");
    const rt = connectRealtime(token, {
      onDelta: (t) => {
        setVoicePhase("speaking");
        setStreaming((x) => x + t);
      },
      onStatus: (s) => setToolStatus(s),
      onDone: () => {
        setStreaming("");
        setVoicePhase("idle");
        if (activeId) void api.listMessages(token, activeId).then(setMessages);
      },
      onTTSChunk: (_seq, _enc, b64) => {
        setVoicePhase("speaking");
        playbackRef.current.enqueueBase64(b64, _enc);
      },
      onConnection: (ok) => setWsConnected(ok),
      onSpeakingIdle: () => {
        setVoicePhase("idle");
        playbackRef.current.stop();
      },
    });
    rtRef.current = rt;
    rt.send({
      version: "1",
      type: "session.start",
      requestId: crypto.randomUUID(),
      sessionId: "",
      timestamp: Date.now(),
      payload: { conversationId: activeId, mode: "voice" },
    });
  }

  function sendVoiceText() {
    if (!input.trim() || !activeId) return;
    setVoicePhase("thinking");
    rtRef.current?.send({
      version: "1",
      type: "text.message",
      requestId: crypto.randomUUID(),
      sessionId: rtRef.current.sessionId || "",
      timestamp: Date.now(),
      payload: { conversationId: activeId, content: input.trim() },
    });
    setInput("");
  }

  async function toggleMic() {
    if (stopMicRef.current) {
      stopMicRef.current();
      stopMicRef.current = null;
      setMicOn(false);
      setVoicePhase("idle");
      rtRef.current?.send({
        version: "1",
        type: "audio.commit",
        requestId: crypto.randomUUID(),
        sessionId: rtRef.current.sessionId || "",
        timestamp: Date.now(),
      });
      return;
    }
    try {
      setVoicePhase("listening");
      setMicOn(true);
      const stop = await startMicStreaming((chunk) => {
        rtRef.current?.send({
          version: "1",
          type: "audio.chunk",
          requestId: crypto.randomUUID(),
          sessionId: rtRef.current?.sessionId || "",
          timestamp: Date.now(),
          payload: {
            sequence: chunk.sequence,
            audioEncoding: chunk.audioEncoding,
            data: chunk.data,
          },
        });
      });
      stopMicRef.current = stop;
    } catch {
      setStatus("microphone permission denied or unavailable");
      setVoicePhase("idle");
      setMicOn(false);
    }
  }

  function interrupt() {
    playbackRef.current.stop();
    setVoicePhase("idle");
    rtRef.current?.send({
      version: "1",
      type: "interrupt",
      requestId: crypto.randomUUID(),
      sessionId: rtRef.current.sessionId || "",
      timestamp: Date.now(),
    });
  }

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      <aside style={{ width: 240, borderRight: "1px solid #e2e8f0", padding: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <strong>EVA</strong>
          <button type="button" onClick={() => logout()}>
            Out
          </button>
        </div>
        <button type="button" onClick={() => void newChat()} style={{ width: "100%", marginTop: 8 }}>
          New chat
        </button>
        <ul style={{ listStyle: "none", padding: 0 }}>
          {conversations.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                onClick={() => setActiveId(c.id)}
                style={{
                  width: "100%",
                  textAlign: "left",
                  marginTop: 4,
                  background: c.id === activeId ? "#e0f2fe" : "transparent",
                  border: "none",
                  padding: 8,
                }}
              >
                {c.title || "Chat"}
              </button>
            </li>
          ))}
        </ul>
        <Link to="/settings">Settings</Link>
      </aside>
      <main style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <div style={{ flex: 1, overflow: "auto", padding: 16 }}>
          {messages.map((m) => (
            <div key={m.id} style={{ marginBottom: 12, opacity: m.role === "tool" ? 0.7 : 1 }}>
              <strong>{m.role}</strong>
              <div style={{ whiteSpace: "pre-wrap" }}>{m.content}</div>
              {m.sources && m.sources.length > 0 && (
                <div style={{ fontSize: 12, color: "#475569" }}>
                  Sources: {m.sources.map((s) => s.title).join(", ")}
                </div>
              )}
            </div>
          ))}
          {streaming && (
            <div style={{ borderLeft: "3px solid #0ea5e9", paddingLeft: 8 }}>
              <strong>assistant (streaming)</strong>
              <div style={{ whiteSpace: "pre-wrap" }}>{streaming}</div>
            </div>
          )}
        </div>
        <div style={{ borderTop: "1px solid #e2e8f0", padding: 8 }}>
          <div style={{ fontSize: 12, color: "#64748b", marginBottom: 6 }}>
            WS: {wsConnected ? "connected" : "disconnected"} · Voice: {voicePhase}
            {status ? ` · ${status}` : ""} {toolStatus}
          </div>
          <form onSubmit={onSend} style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Message…"
              style={{ flex: "1 1 200px", padding: 8, minWidth: 120 }}
            />
            <button type="submit" disabled={!activeId}>
              Send
            </button>
            <button type="button" title="Open WebSocket session" onClick={() => openRealtime()}>
              Voice session
            </button>
            <button type="button" onClick={() => sendVoiceText()} disabled={!rtRef.current}>
              Send via WS
            </button>
            <button
              type="button"
              onClick={() => void toggleMic()}
              style={{ background: micOn ? "#fecaca" : undefined }}
              title="Stream microphone chunks (requires HTTPS)"
            >
              {micOn ? "Stop mic" : "Mic"}
            </button>
            <button type="button" onClick={() => interrupt()}>
              Interrupt
            </button>
          </form>
        </div>
      </main>
    </div>
  );
}
