# EVA — Self-hosted LAN voice assistant

Pet-project stack from `tz-eva.json`: **Go** modular monolith, **React** web client, **OpenAPI**-first HTTP API, **WebSocket** realtime, **Postgres** + **Redis**, **Ollama** for local LLM, **Caddy** for HTTPS (microphone in the browser).

## Quick start

1. Copy [.env.example](.env.example) to `.env` and set a long random `JWT_SECRET`.
2. Generate code (OpenAPI Go + TS types + buf for protos; gRPC Go output is pruned so the monolith only ships the OpenAPI server types):

   ```bash
   make generate
   ```

3. Start stack:

   ```bash
   docker compose -f deploy/compose/docker-compose.yml up --build
   ```

4. Trust Caddy’s local CA (or your own cert) for `https://localhost` — required for mic access in production-like setups.
5. Pull an Ollama model (once), e.g.:

   ```bash
   docker compose -f deploy/compose/docker-compose.yml exec ollama ollama pull llama3.2
   ```

6. Create the first user (empty DB only):

   ```bash
   docker compose -f deploy/compose/docker-compose.yml run --rm --entrypoint /app/seed backend
   ```

   Default login: **admin@local** / **changeme** (change after first login).

7. Open **https://localhost** (via Caddy).

## Local dev (without Docker for backend/web)

- Postgres + Redis + Ollama running locally; set `POSTGRES_DSN`, `REDIS_ADDR`, `LLM_BASE_URL` in `.env`.
- Backend: `cd services/backend && go run ./cmd/server`
- Web: `cd apps/web && npm install && npm run dev` (Vite proxies `/api` and `/ws` to `:8080`).

## Endpoints

- REST: `/api/v1/...` (see [openapi/openapi.yaml](openapi/openapi.yaml))
- OpenAPI JSON: `/api/openapi.json`
- WebSocket: `/ws/v1/realtime?token=<JWT>`
- Prometheus: `/metrics`

## Layout

- [openapi/](openapi/) — HTTP contract
- [proto/](proto/) — gRPC contracts (`make proto` regenerates Go stubs under `services/backend/internal/gen` when needed)
- [services/backend/](services/backend/) — Go server
- [apps/web/](apps/web/) — Vite + React UI
- [packages/ts-client-generated/](packages/ts-client-generated/) — `schema.d.ts` from OpenAPI
- [deploy/](deploy/) — Compose + Caddy

## Voice / search notes

- **TTS/STT** default to `noop` providers; WebSocket protocol is implemented so you can plug in real services later.
- With **`TTS_PROVIDER=noop`** and **`TTS_NOOP_BEEP=true`** (default), the server sends a short **WAV** `tts.chunk` after each WS reply so the web client can verify **Web Audio** playback end-to-end. Set `TTS_NOOP_BEEP=false` to disable the tone.
- **Search** uses DuckDuckGo’s JSON API (no API key); failures are surfaced to the model and user honestly.

## Future

- Desktop/mobile shells: [apps/desktop_placeholder/](apps/desktop_placeholder/), [apps/mobile_placeholder/](apps/mobile_placeholder/)
- Kafka-style events (documented only): [docs/kafka-future.md](docs/kafka-future.md)
