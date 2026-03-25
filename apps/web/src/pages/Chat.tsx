import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import * as api from "../api/client";
import type { Message } from "../api/client";
import { useAuth } from "../auth";
import { connectRealtime, Realtime } from "../realtime/ws";

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
  const rtRef = useRef<Realtime | null>(null);

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
          const msgs = await api.listMessages(token, activeId);
          setMessages(msgs);
          void final;
        }
      );
    } catch {
      setStatus("chat error");
    }
  }

  function startVoiceSession() {
    if (!activeId || !token) return;
    rtRef.current?.close();
    const rt = connectRealtime(token, {
      onDelta: (t) => setStreaming((x) => x + t),
      onStatus: (s) => setToolStatus(s),
      onDone: () => {
        setStreaming("");
        api.listMessages(token, activeId).then(setMessages);
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

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      <aside style={{ width: 240, borderRight: "1px solid #e2e8f0", padding: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <strong>EVA</strong>
          <button type="button" onClick={() => logout()}>
            Out
          </button>
        </div>
        <button type="button" onClick={() => newChat()} style={{ width: "100%", marginTop: 8 }}>
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
          <div style={{ fontSize: 12, color: "#64748b" }}>
            {status} {toolStatus}
          </div>
          <form onSubmit={onSend} style={{ display: "flex", gap: 8 }}>
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Message…"
              style={{ flex: 1, padding: 8 }}
            />
            <button type="submit" disabled={!activeId}>
              Send
            </button>
            <button type="button" onClick={() => startVoiceSession()}>
              WS session
            </button>
            <button type="button" onClick={() => sendVoiceText()}>
              WS send
            </button>
            <button
              type="button"
              onClick={() => {
                rtRef.current?.send({
                  version: "1",
                  type: "interrupt",
                  requestId: crypto.randomUUID(),
                  sessionId: rtRef.current.sessionId || "",
                  timestamp: Date.now(),
                });
              }}
            >
              Interrupt
            </button>
          </form>
        </div>
      </main>
    </div>
  );
}
