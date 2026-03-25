import type { paths } from "../../../../packages/ts-client-generated/schema";

export type TokenPair = {
  accessToken: string;
  refreshToken: string;
  __ts?: never;
};

const origFetch = globalThis.fetch.bind(globalThis);

export function apiBase(): string {
  if (import.meta.env.DEV) return "";
  return "";
}

export async function apiFetch(
  path: string,
  init: RequestInit & { token?: string | null } = {}
): Promise<Response> {
  const { token, headers: h, ...rest } = init;
  const headers = new Headers(h);
  if (!headers.has("Content-Type") && rest.body && typeof rest.body === "string") {
    headers.set("Content-Type", "application/json");
  }
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return origFetch(`${apiBase()}${path}`, { ...rest, headers });
}

export type LoginBody = paths["/api/v1/auth/login"]["post"]["requestBody"]["content"]["application/json"];
export type LoginRes = paths["/api/v1/auth/login"]["post"]["responses"]["200"]["content"]["application/json"];

export async function login(body: LoginBody): Promise<LoginRes> {
  const r = await apiFetch("/api/v1/auth/login", { method: "POST", body: JSON.stringify(body) });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export type Settings = paths["/api/v1/settings"]["get"]["responses"]["200"]["content"]["application/json"];
export type PatchSettings = paths["/api/v1/settings"]["patch"]["requestBody"]["content"]["application/json"];

export async function getSettings(token: string): Promise<Settings> {
  const r = await apiFetch("/api/v1/settings", { token });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export async function patchSettings(token: string, body: PatchSettings): Promise<Settings> {
  const r = await apiFetch("/api/v1/settings", {
    method: "PATCH",
    token,
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export type Conversation =
  paths["/api/v1/conversations"]["get"]["responses"]["200"]["content"]["application/json"]["conversations"][number];

export async function listConversations(token: string): Promise<Conversation[]> {
  const r = await apiFetch("/api/v1/conversations", { token });
  if (!r.ok) throw new Error(await r.text());
  const j = await r.json();
  return j.conversations ?? [];
}

export async function createConversation(token: string, title?: string): Promise<Conversation> {
  const r = await apiFetch("/api/v1/conversations", {
    method: "POST",
    token,
    body: JSON.stringify(title ? { title } : {}),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export type Message =
  paths["/api/v1/conversations/{conversationId}/messages"]["get"]["responses"]["200"]["content"]["application/json"]["messages"][number];

export async function listMessages(token: string, conversationId: string): Promise<Message[]> {
  const r = await apiFetch(`/api/v1/conversations/${conversationId}/messages`, { token });
  if (!r.ok) throw new Error(await r.text());
  const j = await r.json();
  return j.messages ?? [];
}

export type ChatReq = paths["/api/v1/chat/messages"]["post"]["requestBody"]["content"]["application/json"];
export type ChatRes = paths["/api/v1/chat/messages"]["post"]["responses"]["200"]["content"]["application/json"];

export async function chatMessage(token: string, body: ChatReq): Promise<ChatRes> {
  const r = await apiFetch("/api/v1/chat/messages", {
    method: "POST",
    token,
    body: JSON.stringify({ ...body, stream: false }),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export async function chatMessageStream(
  token: string,
  body: ChatReq,
  onDelta: (t: string) => void,
  onDone: (reply: string) => void
): Promise<void> {
  const r = await apiFetch("/api/v1/chat/messages", {
    method: "POST",
    token,
    body: JSON.stringify({ conversationId: body.conversationId, content: body.content, stream: true }),
  });
  if (!r.ok) throw new Error(await r.text());
  const reader = r.body?.getReader();
  if (!reader) throw new Error("no body");
  const dec = new TextDecoder();
  let buf = "";
  let full = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    const parts = buf.split("\n\n");
    buf = parts.pop() ?? "";
    for (const block of parts) {
      const lines = block.split("\n");
      let event = "";
      let data = "";
      for (const line of lines) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (event === "delta" && data) {
        try {
          const j = JSON.parse(data) as { type?: string; text?: string };
          if (j.text) {
            full += j.text;
            onDelta(j.text);
          }
        } catch {
          /* ignore */
        }
      }
      if (event === "done" && data) {
        try {
          const j = JSON.parse(data) as { reply?: string };
          if (j.reply) full = j.reply;
        } catch {
          /* ignore */
        }
      }
    }
  }
  onDone(full);
}
