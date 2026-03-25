package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

type User struct {
	ID              uuid.UUID
	EmailOrUsername string
	PasswordHash    string
}

func (s *Store) CreateUser(ctx context.Context, emailOrUsername, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (email_or_username, password_hash)
		VALUES ($1, $2)
		RETURNING id, email_or_username, password_hash
	`, emailOrUsername, string(hash)).Scan(&u.ID, &u.EmailOrUsername, &u.PasswordHash)
	return u, err
}

func (s *Store) GetUserByLogin(ctx context.Context, emailOrUsername string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email_or_username, password_hash FROM users WHERE email_or_username = $1
	`, emailOrUsername).Scan(&u.ID, &u.EmailOrUsername, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, pgx.ErrNoRows
	}
	return u, err
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email_or_username, password_hash FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.EmailOrUsername, &u.PasswordHash)
	return u, err
}

func (s *Store) VerifyPassword(u User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *Store) SaveRefreshToken(ctx context.Context, userID uuid.UUID, raw string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
	`, userID, hashToken(raw), expiresAt)
	return err
}

func (s *Store) RotateRefreshToken(ctx context.Context, raw string) (uuid.UUID, string, time.Time, error) {
	hash := hashToken(raw)
	var userID uuid.UUID
	var exp time.Time
	err := s.pool.QueryRow(ctx, `
		DELETE FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id, expires_at
	`, hash).Scan(&userID, &exp)
	if err != nil {
		return uuid.Nil, "", time.Time{}, err
	}
	newRaw := make([]byte, 32)
	if _, err := rand.Read(newRaw); err != nil {
		return uuid.Nil, "", time.Time{}, err
	}
	newStr := hex.EncodeToString(newRaw)
	newExp := time.Now().Add(7 * 24 * time.Hour)
	if err := s.SaveRefreshToken(ctx, userID, newStr, newExp); err != nil {
		return uuid.Nil, "", time.Time{}, err
	}
	return userID, newStr, newExp, nil
}

type Settings struct {
	UserID        uuid.UUID
	Language      string
	Theme         string
	VoiceEnabled  bool
	AutoPlayVoice bool
	VoiceName     *string
	VoiceLanguage string
	SpeechRate    float64
	SpeechVolume  float64
	DefaultModel  *string
	STTProvider   *string
	TTSProvider   *string
}

func (s *Store) EnsureSettings(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	return err
}

func (s *Store) GetSettings(ctx context.Context, userID uuid.UUID) (Settings, error) {
	_ = s.EnsureSettings(ctx, userID)
	var st Settings
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, language, theme, voice_enabled, auto_play_voice, voice_name, voice_language,
		       speech_rate, speech_volume, default_model, stt_provider, tts_provider
		FROM user_settings WHERE user_id = $1
	`, userID).Scan(
		&st.UserID, &st.Language, &st.Theme, &st.VoiceEnabled, &st.AutoPlayVoice,
		&st.VoiceName, &st.VoiceLanguage, &st.SpeechRate, &st.SpeechVolume,
		&st.DefaultModel, &st.STTProvider, &st.TTSProvider,
	)
	st.UserID = userID
	return st, err
}

func (s *Store) UpdateSettings(ctx context.Context, userID uuid.UUID, u Settings) (Settings, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_settings SET
		  language = $2, theme = $3, voice_enabled = $4, auto_play_voice = $5,
		  voice_name = $6, voice_language = $7, speech_rate = $8, speech_volume = $9,
		  default_model = $10, stt_provider = $11, tts_provider = $12, updated_at = now()
		WHERE user_id = $1
	`, userID, u.Language, u.Theme, u.VoiceEnabled, u.AutoPlayVoice,
		u.VoiceName, u.VoiceLanguage, u.SpeechRate, u.SpeechVolume,
		u.DefaultModel, u.STTProvider, u.TTSProvider,
	)
	if err != nil {
		return Settings{}, err
	}
	u.UserID = userID
	return u, nil
}

type Conversation struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) ListConversations(ctx context.Context, userID uuid.UUID) ([]Conversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, title, created_at, updated_at FROM conversations
		WHERE user_id = $1 ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateConversation(ctx context.Context, userID uuid.UUID, title string) (Conversation, error) {
	var c Conversation
	err := s.pool.QueryRow(ctx, `
		INSERT INTO conversations (user_id, title) VALUES ($1, $2)
		RETURNING id, user_id, title, created_at, updated_at
	`, userID, title).Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) GetConversation(ctx context.Context, userID, convID uuid.UUID) (Conversation, error) {
	var c Conversation
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, created_at, updated_at FROM conversations
		WHERE id = $1 AND user_id = $2
	`, convID, userID).Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Role           string
	Content        string
	Sources        []byte // json
	HasTtsAudio    bool
	CreatedAt      time.Time
}

func (s *Store) ListMessages(ctx context.Context, userID, convID uuid.UUID) ([]Message, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE id = $1 AND user_id = $2`, convID, userID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, pgx.ErrNoRows
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, conversation_id, role, content, sources_json, has_tts_audio, created_at
		FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC
	`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sources, &m.HasTtsAudio, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(ctx context.Context, convID uuid.UUID, role, content string, sources json.RawMessage) (Message, error) {
	var m Message
	err := s.pool.QueryRow(ctx, `
		INSERT INTO messages (conversation_id, role, content, sources_json)
		VALUES ($1, $2, $3, $4)
		RETURNING id, conversation_id, role, content, sources_json, has_tts_audio, created_at
	`, convID, role, content, sources).Scan(
		&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sources, &m.HasTtsAudio, &m.CreatedAt,
	)
	if err == nil {
		_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, convID)
	}
	return m, err
}

func (s *Store) RecordToolExecution(ctx context.Context, convID uuid.UUID, name, status string, in, out json.RawMessage, errMsg *string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tool_executions (conversation_id, tool_name, status, input_json, output_json, error_message)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, convID, name, status, in, out, errMsg)
	return err
}

func (s *Store) UserCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}
