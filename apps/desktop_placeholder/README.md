# Desktop (future)

Target shell: **Tauri 2.x** with the existing Vite app:

1. `npm create tauri-app` (or add Tauri to `apps/web`) with the same `src/` and route to `https://localhost` or a configurable `PUBLIC_BASE_URL`.
2. Reuse [`packages/ts-client-generated`](../packages/ts-client-generated/) and the same adapters as web:
   - `src/realtime/ws.ts` — replace `WebSocket` URL with `wss://` to your LAN host.
   - `src/audio/mic.ts` / `src/audio/playback.ts` — same Web APIs in Tauri’s WebView2/WebKit.
3. Optional native hooks later: global hotkey, tray, auto-start Ollama — keep business logic in shared TS, not in Rust shims.

No desktop binary is built in this repo yet; the web app is the single shipped UI.
