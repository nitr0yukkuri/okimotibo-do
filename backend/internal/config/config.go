package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port              string
	AllowedOrigins    []string
	AllowAnonymous    bool
	SupabaseURL       string
	SupabaseSecretKey string
	HTTPTimeout       time.Duration
	StatusTTL         time.Duration
}

func FromEnv() (Config, error) {
	statusTTL, err := time.ParseDuration(envOr("STATUS_TTL", "15m"))
	if err != nil || statusTTL < 0 || statusTTL > 24*time.Hour {
		return Config{}, errors.New("STATUS_TTL must be 0 (no expiry) or a duration up to 24h")
	}
	cfg := Config{
		Port:              envOr("PORT", "8080"),
		AllowedOrigins:    splitCSV(envOr("ALLOWED_ORIGINS", "http://localhost:5173")),
		AllowAnonymous:    strings.EqualFold(os.Getenv("ALLOW_ANONYMOUS"), "true"),
		SupabaseURL:       strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		SupabaseSecretKey: os.Getenv("SUPABASE_SECRET_KEY"),
		HTTPTimeout:       5 * time.Second,
		StatusTTL:         statusTTL,
	}
	if (cfg.SupabaseURL == "") != (cfg.SupabaseSecretKey == "") {
		return Config{}, errors.New("SUPABASE_URL and SUPABASE_SECRET_KEY must be set together")
	}
	if !cfg.AllowAnonymous && cfg.SupabaseURL == "" {
		return Config{}, errors.New("Supabase credentials are required unless ALLOW_ANONYMOUS=true")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
