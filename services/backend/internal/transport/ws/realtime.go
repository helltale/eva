package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"eva/services/backend/internal/assistant"
	"eva/services/backend/internal/auth"
	"eva/services/backend/internal/config"
	"eva/services/backend/internal/observability"
	"eva/services/backend/internal/voice/nooptts"
)

type Envelope struct {
	Version   string          `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	SessionID string          `json:"sessionId"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type Handler struct {
	cfg    config.Config
	runner *assistant.Runner
	authFn func(token string) (uuid.UUID, error)
	sess   sync.Map // sessionID string -> *sessionState
}

type sessionState struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewHandler(cfg config.Config, runner *assistant.Runner) *Handler {
	return &Handler{
		cfg:    cfg,
		runner: runner,
		authFn: func(token string) (uuid.UUID, error) {
			return auth.ParseAccess(cfg.JWTSecret, token)
		},
	}
}

func (h *Handler) allowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	for _, o := range h.cfg.CORSOrigins {
		if o == origin || o == "*" {
			return true
		}
	}
	return false
}

// ServeHTTP upgrades to WebSocket; token via query ?token=
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	if tok == "" {
		tok = auth.BearerToken(r)
	}
	uid, err := h.authFn(tok)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(req *http.Request) bool {
			og := req.Header.Get("Origin")
			if og == "" {
				return true
			}
			return h.allowedOrigin(og)
		},
	}
	c, err := u.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	var writeMu sync.Mutex
	write := func(env Envelope) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = c.WriteJSON(env)
	}

	observability.WSSessions.Inc()
	defer observability.WSSessions.Dec()

	ctx := r.Context()
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		var env Envelope
		if json.Unmarshal(data, &env) != nil {
			sendErr(write, &Envelope{Version: "1"}, "bad_envelope", "invalid json")
			continue
		}
		if env.Version == "" {
			env.Version = "1"
		}
		switch env.Type {
		case "ping":
			write(Envelope{Version: env.Version, Type: "pong", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli()})
		case "session.start":
			write(Envelope{Version: env.Version, Type: "session.started", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli()})
		case "session.stop":
			h.handleSessionStop(env.SessionID)
			write(Envelope{Version: env.Version, Type: "session.stopped", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli()})
		case "text.message":
			h.handleTextMessage(ctx, write, uid, env)
		case "audio.chunk":
			observability.STTRequests.Inc()
			write(Envelope{Version: env.Version, Type: "stt.partial", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli(), Payload: mustJSON(map[string]string{"text": ""})})
		case "audio.commit":
			write(Envelope{Version: env.Version, Type: "stt.final", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli(), Payload: mustJSON(map[string]string{"text": ""})})
		case "interrupt":
			observability.VoiceInterrupts.Inc()
			h.cancelSession(env.SessionID)
			write(Envelope{Version: env.Version, Type: "tts.finished", RequestID: env.RequestID, SessionID: env.SessionID, Timestamp: time.Now().UnixMilli()})
		case "tts.stop":
			h.cancelSession(env.SessionID)
		default:
			sendErr(write, &env, "unknown_type", env.Type)
		}
	}
}

func sendErr(write func(Envelope), env *Envelope, code, msg string) {
	write(Envelope{
		Version:   "1",
		Type:      "error",
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		Timestamp: time.Now().UnixMilli(),
		Payload:   mustJSON(map[string]string{"code": code, "message": msg}),
	})
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (h *Handler) handleSessionStop(sid string) {
	h.cancelSession(sid)
	h.sess.Delete(sid)
}

func (h *Handler) cancelSession(sid string) {
	if sid == "" {
		return
	}
	if v, ok := h.sess.Load(sid); ok {
		st := v.(*sessionState)
		st.mu.Lock()
		if st.cancel != nil {
			st.cancel()
		}
		st.mu.Unlock()
	}
}

func (h *Handler) handleTextMessage(parent context.Context, write func(Envelope), uid uuid.UUID, env Envelope) {
	var p struct {
		ConversationID string `json:"conversationId"`
		Content        string `json:"content"`
	}
	if json.Unmarshal(env.Payload, &p) != nil {
		sendErr(write, &env, "bad_payload", "text.message payload")
		return
	}
	cid, err := uuid.Parse(p.ConversationID)
	if err != nil {
		sendErr(write, &env, "bad_payload", "conversationId")
		return
	}
	sid := env.SessionID
	if sid == "" {
		sid = uuid.NewString()
	}
	h.cancelSession(sid)
	ctx, cancel := context.WithCancel(parent)
	st := &sessionState{cancel: cancel}
	h.sess.Store(sid, st)

	go func() {
		defer cancel()
		reply, _, _, err := h.runner.RunTurn(ctx, uid, cid, p.Content)
		if err != nil {
			sendErr(write, &env, "assistant_error", err.Error())
			return
		}
		for _, part := range assistant.ChunkText(reply) {
			if ctx.Err() != nil {
				return
			}
			write(Envelope{
				Version:   env.Version,
				Type:      "assistant.delta",
				RequestID: env.RequestID,
				SessionID: sid,
				Timestamp: time.Now().UnixMilli(),
				Payload:   mustJSON(map[string]string{"text": part}),
			})
		}
		write(Envelope{
			Version:   env.Version,
			Type:      "assistant.message",
			RequestID: env.RequestID,
			SessionID: sid,
			Timestamp: time.Now().UnixMilli(),
			Payload:   mustJSON(map[string]any{"conversationId": cid.String(), "message": map[string]string{"role": "assistant", "content": reply}}),
		})
		write(Envelope{Version: env.Version, Type: "tts.started", RequestID: env.RequestID, SessionID: sid, Timestamp: time.Now().UnixMilli(), Payload: mustJSON(map[string]string{"voiceName": "default"})})
		observability.TTSRequests.Inc()
		t0 := time.Now()
		if h.cfg.TTSNoopBeep && h.cfg.TTSProvider == "noop" {
			seq, enc, b64 := nooptts.WSChunk()
			write(Envelope{
				Version:   env.Version,
				Type:      "tts.chunk",
				RequestID: env.RequestID,
				SessionID: sid,
				Timestamp: time.Now().UnixMilli(),
				Payload: mustJSON(map[string]any{
					"sequence":      seq,
					"audioEncoding": enc,
					"data":          b64,
				}),
			})
		}
		observability.TTSDuration.Observe(time.Since(t0).Seconds())
		write(Envelope{Version: env.Version, Type: "tts.finished", RequestID: env.RequestID, SessionID: sid, Timestamp: time.Now().UnixMilli()})
	}()
}
