package httptransport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"eva/services/backend/internal/assistant"
	"eva/services/backend/internal/auth"
	"eva/services/backend/internal/config"
	"eva/services/backend/internal/gen/openapi"
	"eva/services/backend/internal/llm"
	"eva/services/backend/internal/observability"
	"eva/services/backend/internal/repository"
	"eva/services/backend/internal/search"
	"eva/services/backend/internal/spec"
)

// Server implements openapi.ServerInterface.
type Server struct {
	cfg         config.Config
	store       *repository.Store
	redis       *redis.Client
	llm         *llm.Client
	search      *search.Client
	runner      *assistant.Runner
	openAPIJSON []byte
}

// Runner exposes assistant for WebSocket transport.
func (s *Server) Runner() *assistant.Runner { return s.runner }

func NewServer(cfg config.Config, store *repository.Store, rdb *redis.Client) (*Server, error) {
	var doc any
	if err := yaml.Unmarshal(spec.OpenAPIYAML, &doc); err != nil {
		return nil, fmt.Errorf("openapi yaml: %w", err)
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	lc := &llm.Client{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
		HTTPClient: &http.Client{
			Timeout: cfg.LLMTimeout,
		},
	}
	sc := &search.Client{}
	s := &Server{
		cfg:         cfg,
		store:       store,
		redis:       rdb,
		llm:         lc,
		search:      sc,
		openAPIJSON: js,
		runner:      &assistant.Runner{LLM: lc, Search: sc, Store: store},
	}
	return s, nil
}

func errJSON(w http.ResponseWriter, code int, c, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(openapi.ErrorResponse{Code: c, Message: msg})
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	tok := auth.BearerToken(r)
	if tok == "" {
		errJSON(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return uuid.Nil, false
	}
	uid, err := auth.ParseAccess(s.cfg.JWTSecret, tok)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return uuid.Nil, false
	}
	return uid, true
}

func (s *Server) GetOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.openAPIJSON)
}

func (s *Server) HealthLive(w http.ResponseWriter, r *http.Request) {
	observability.HTTPInFlight.Inc()
	defer observability.HTTPInFlight.Dec()
	w.Header().Set("Content-Type", "application/json")
	st := openapi.HealthResponseStatusOk
	_ = json.NewEncoder(w).Encode(openapi.HealthResponse{Status: st})
}

func (s *Server) HealthReady(w http.ResponseWriter, r *http.Request) {
	observability.HTTPInFlight.Inc()
	defer observability.HTTPInFlight.Dec()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]openapi.HealthResponseChecks{}
	st := openapi.HealthResponseStatusOk
	if err := s.store.Ping(ctx); err != nil {
		checks["postgres"] = openapi.HealthResponseChecksFail
		st = openapi.HealthResponseStatusDegraded
	} else {
		checks["postgres"] = openapi.HealthResponseChecksOk
	}
	if s.redis != nil {
		if err := s.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = openapi.HealthResponseChecksFail
			st = openapi.HealthResponseStatusDegraded
		} else {
			checks["redis"] = openapi.HealthResponseChecksOk
		}
	}
	root := strings.TrimSuffix(strings.TrimSuffix(s.cfg.LLMBaseURL, "/"), "/v1")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, root+"/api/tags", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode >= 500 {
		checks["llm"] = openapi.HealthResponseChecksFail
		st = openapi.HealthResponseStatusDegraded
	} else {
		checks["llm"] = openapi.HealthResponseChecksOk
		if res != nil {
			res.Body.Close()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if st == openapi.HealthResponseStatusDegraded {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(openapi.HealthResponse{
		Status: st,
		Checks: &checks,
	})
}

func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) {
	var body openapi.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	u, err := s.store.GetUserByLogin(r.Context(), body.EmailOrUsername)
	if err != nil || !s.store.VerifyPassword(u, body.Password) {
		errJSON(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	_ = s.store.EnsureSettings(r.Context(), u.ID)
	access, err := auth.IssueAccess(s.cfg.JWTSecret, u.ID, 15*time.Minute)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "token")
		return
	}
	raw := uuid.NewString() + uuid.NewString()
	if err := s.store.SaveRefreshToken(r.Context(), u.ID, raw, time.Now().Add(7*24*time.Hour)); err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "refresh")
		return
	}
	writeLogin(w, u, access, raw)
}

func writeLogin(w http.ResponseWriter, u repository.User, access, refresh string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openapi.LoginResponse{
		User: openapi.UserProfile{
			Id:               openapi_types.UUID(u.ID),
			EmailOrUsername: u.EmailOrUsername,
		},
		Tokens: openapi.TokenPair{
			AccessToken:  access,
			RefreshToken: refresh,
			ExpiresIn:    900,
		},
	})
}

func (s *Server) AuthRefresh(w http.ResponseWriter, r *http.Request) {
	var body openapi.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	uid, newRefresh, _, err := s.store.RotateRefreshToken(r.Context(), body.RefreshToken)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "unauthorized", "invalid refresh")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), uid)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "unauthorized", "user")
		return
	}
	access, err := auth.IssueAccess(s.cfg.JWTSecret, uid, 15*time.Minute)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "token")
		return
	}
	writeLogin(w, u, access, newRefresh)
}

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	u, err := s.store.GetUserByID(r.Context(), uid)
	if err != nil {
		errJSON(w, http.StatusNotFound, "not_found", "user")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openapi.UserProfile{
		Id:               openapi_types.UUID(u.ID),
		EmailOrUsername: u.EmailOrUsername,
	})
}

func toAPISettings(st repository.Settings) openapi.UserSettings {
	return openapi.UserSettings{
		Language:      st.Language,
		Theme:         openapi.UserSettingsTheme(st.Theme),
		VoiceEnabled:  st.VoiceEnabled,
		AutoPlayVoice: st.AutoPlayVoice,
		VoiceName:     st.VoiceName,
		VoiceLanguage: st.VoiceLanguage,
		SpeechRate:    float32(st.SpeechRate),
		SpeechVolume:  float32(st.SpeechVolume),
		DefaultModel:  st.DefaultModel,
		SttProvider:   st.STTProvider,
		TtsProvider:   st.TTSProvider,
	}
}

func (s *Server) GetSettings(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	st, err := s.store.GetSettings(r.Context(), uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "settings")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAPISettings(st))
}

func (s *Server) PatchSettings(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var patch openapi.UpdateUserSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		errJSON(w, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	cur, err := s.store.GetSettings(r.Context(), uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "settings")
		return
	}
	if patch.Language != nil {
		cur.Language = *patch.Language
	}
	if patch.Theme != nil {
		cur.Theme = string(*patch.Theme)
	}
	if patch.VoiceEnabled != nil {
		cur.VoiceEnabled = *patch.VoiceEnabled
	}
	if patch.AutoPlayVoice != nil {
		cur.AutoPlayVoice = *patch.AutoPlayVoice
	}
	if patch.VoiceName != nil {
		cur.VoiceName = patch.VoiceName
	}
	if patch.VoiceLanguage != nil {
		cur.VoiceLanguage = *patch.VoiceLanguage
	}
	if patch.SpeechRate != nil {
		cur.SpeechRate = float64(*patch.SpeechRate)
	}
	if patch.SpeechVolume != nil {
		cur.SpeechVolume = float64(*patch.SpeechVolume)
	}
	if patch.DefaultModel != nil {
		cur.DefaultModel = patch.DefaultModel
	}
	if patch.SttProvider != nil {
		cur.STTProvider = patch.SttProvider
	}
	if patch.TtsProvider != nil {
		cur.TTSProvider = patch.TtsProvider
	}
	st, err := s.store.UpdateSettings(r.Context(), uid, cur)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "update")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAPISettings(st))
}

func (s *Server) ListConversations(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListConversations(r.Context(), uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "list")
		return
	}
	out := make([]openapi.Conversation, 0, len(list))
	for _, c := range list {
		out = append(out, openapi.Conversation{
			Id:        openapi_types.UUID(c.ID),
			UserId:    openapi_types.UUID(c.UserID),
			Title:     c.Title,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openapi.ConversationListResponse{Conversations: out})
}

func (s *Server) CreateConversation(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body openapi.CreateConversationRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	title := ""
	if body.Title != nil {
		title = *body.Title
	}
	c, err := s.store.CreateConversation(r.Context(), uid, title)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server_error", "create")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(openapi.Conversation{
		Id:        openapi_types.UUID(c.ID),
		UserId:    openapi_types.UUID(c.UserID),
		Title:     c.Title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	})
}

func (s *Server) GetConversation(w http.ResponseWriter, r *http.Request, conversationId openapi_types.UUID) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	c, err := s.store.GetConversation(r.Context(), uid, uuid.UUID(conversationId))
	if err != nil {
		errJSON(w, http.StatusNotFound, "not_found", "conversation")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openapi.Conversation{
		Id:        openapi_types.UUID(c.ID),
		UserId:    openapi_types.UUID(c.UserID),
		Title:     c.Title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	})
}

func (s *Server) ListMessages(w http.ResponseWriter, r *http.Request, conversationId openapi_types.UUID) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	msgs, err := s.store.ListMessages(r.Context(), uid, uuid.UUID(conversationId))
	if err != nil {
		errJSON(w, http.StatusNotFound, "not_found", "messages")
		return
	}
	out := make([]openapi.Message, 0, len(msgs))
	for _, m := range msgs {
		om := openapi.Message{
			Id:             openapi_types.UUID(m.ID),
			ConversationId: openapi_types.UUID(m.ConversationID),
			Role:           openapi.MessageRole(m.Role),
			Content:        m.Content,
			HasTtsAudio:    &m.HasTtsAudio,
			CreatedAt:      m.CreatedAt,
		}
		if len(m.Sources) > 0 {
			var src []openapi.MessageSource
			if json.Unmarshal(m.Sources, &src) == nil {
				om.Sources = &src
			}
		}
		out = append(out, om)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openapi.MessageListResponse{Messages: out})
}

func (s *Server) ChatMessage(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body openapi.ChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	cid := uuid.UUID(body.ConversationId)
	if _, err := s.store.GetConversation(r.Context(), uid, cid); err != nil {
		errJSON(w, http.StatusNotFound, "not_found", "conversation")
		return
	}
	stream := body.Stream != nil && *body.Stream
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fl, ok := w.(http.Flusher)
		if !ok {
			errJSON(w, http.StatusInternalServerError, "server_error", "stream")
			return
		}
		// Run full turn (tools + LLM) then chunk via SSE for progressive UI.
		reply, sources, tools, err := s.runner.RunTurn(r.Context(), uid, cid, body.Content)
		if err != nil {
			payload, _ := json.Marshal(map[string]string{"error": err.Error()})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
			fl.Flush()
			return
		}
		for _, part := range assistant.ChunkText(reply) {
			d, _ := json.Marshal(map[string]any{"type": "assistant.delta", "text": part})
			_, _ = fmt.Fprintf(w, "event: delta\ndata: %s\n\n", d)
			fl.Flush()
		}
		donePayload := map[string]any{"type": "assistant.message", "conversationId": cid.String(), "reply": reply}
		if len(sources) > 0 {
			donePayload["sources"] = json.RawMessage(sources)
		}
		if len(tools) > 0 {
			donePayload["tools"] = tools
		}
		d, _ := json.Marshal(donePayload)
		_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", d)
		fl.Flush()
		return
	}
	_, sources, tools, err := s.runner.RunTurn(r.Context(), uid, cid, body.Content)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "assistant_error", err.Error())
		return
	}
	msgs, err := s.store.ListMessages(r.Context(), uid, cid)
	if err != nil || len(msgs) == 0 {
		errJSON(w, http.StatusInternalServerError, "server_error", "messages")
		return
	}
	last := msgs[len(msgs)-1]
	texec := toolExecOpenAPI(tools)
	resp := openapi.ChatMessageResponse{
		Message: openapi.Message{
			Id:             openapi_types.UUID(last.ID),
			ConversationId: openapi_types.UUID(last.ConversationID),
			Role:           openapi.MessageRole(last.Role),
			Content:        last.Content,
			CreatedAt:      last.CreatedAt,
		},
	}
	if len(sources) > 0 {
		var src []openapi.MessageSource
		if json.Unmarshal(sources, &src) == nil {
			resp.Message.Sources = &src
		}
	}
	if texec != nil {
		resp.ToolExecutions = texec
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func toolExecOpenAPI(tools []assistant.ToolExecInfo) *[]struct {
	Status   *string `json:"status,omitempty"`
	ToolName *string `json:"toolName,omitempty"`
} {
	if len(tools) == 0 {
		return nil
	}
	out := make([]struct {
		Status   *string `json:"status,omitempty"`
		ToolName *string `json:"toolName,omitempty"`
	}, len(tools))
	for i := range tools {
		tn := tools[i].ToolName
		st := tools[i].Status
		out[i].ToolName = &tn
		out[i].Status = &st
	}
	return &out
}

func (s *Server) ListTools(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	_ = uid
	tools := []openapi.ToolDescriptor{
		{Name: "web_search", Description: "Search the web for up-to-date information. Invoked automatically when the model chooses to call it."},
	}
	w.Header().Set("Content-Type", "application/json")
	type out struct {
		Tools []openapi.ToolDescriptor `json:"tools"`
	}
	_ = json.NewEncoder(w).Encode(out{tools})
}

func (s *Server) ListVoices(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	_ = uid
	voices := openapi.VoiceProfileListResponse{
		Voices: []openapi.VoiceProfile{
			{Id: "default", Name: "Default", Language: "en", Provider: ptr("noop")},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(voices)
}

func ptr[T any](v T) *T { return &v }
