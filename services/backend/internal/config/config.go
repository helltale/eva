package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv           string
	Host             string
	Port             string
	PublicBaseURL    string
	JWTSecret        string
	PostgresDSN      string
	RedisAddr        string
	CORSOrigins      []string
	LogLevel         string
	MigrationsDir    string
	OpenAPIPath      string
	LLMProvider      string
	LLMBaseURL       string
	LLMModel         string
	LLMAPIKey        string
	LLMTimeout       time.Duration
	SearchProvider   string
	SearchBaseURL    string
	STTProvider      string
	STTBaseURL       string
	TTSProvider      string
	TTSBaseURL       string
	DefaultVoiceName string
}

func Load() (Config, error) {
	c := Config{
		AppEnv:           getenv("APP_ENV", "development"),
		Host:             getenv("APP_HOST", "0.0.0.0"),
		Port:             getenv("APP_PORT", "8080"),
		PublicBaseURL:    getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		PostgresDSN:      os.Getenv("POSTGRES_DSN"),
		RedisAddr:        getenv("REDIS_ADDR", "localhost:6379"),
		LogLevel:         getenv("LOG_LEVEL", "info"),
		MigrationsDir:    getenv("APP_MIGRATIONS_DIR", "./migrations"),
		OpenAPIPath:      getenv("OPENAPI_PATH", "assets/openapi.yaml"),
		LLMProvider:      getenv("LLM_PROVIDER", "openai_compat"),
		LLMBaseURL:       getenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1"),
		LLMModel:         getenv("LLM_MODEL", "llama3.2"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		SearchProvider:   getenv("SEARCH_PROVIDER", "duckduckgo_json"),
		SearchBaseURL:    os.Getenv("SEARCH_BASE_URL"),
		STTProvider:      getenv("STT_PROVIDER", "noop"),
		STTBaseURL:       os.Getenv("STT_BASE_URL"),
		TTSProvider:      getenv("TTS_PROVIDER", "noop"),
		TTSBaseURL:       os.Getenv("TTS_BASE_URL"),
		DefaultVoiceName: getenv("DEFAULT_VOICE_NAME", "default"),
	}
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				c.CORSOrigins = append(c.CORSOrigins, o)
			}
		}
	} else {
		c.CORSOrigins = []string{c.PublicBaseURL}
	}
	if c.JWTSecret == "" {
		return c, fmt.Errorf("JWT_SECRET is required")
	}
	if c.PostgresDSN == "" {
		return c, fmt.Errorf("POSTGRES_DSN is required")
	}
	sec, _ := strconv.Atoi(getenv("LLM_TIMEOUT_SECONDS", "120"))
	if sec <= 0 {
		sec = 120
	}
	c.LLMTimeout = time.Duration(sec) * time.Second
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
