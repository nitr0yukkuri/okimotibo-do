package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaultsToFifteenMinuteStatusTTL(t *testing.T) {
	t.Setenv("STATUS_TTL", "")
	t.Setenv("ALLOW_ANONYMOUS", "true")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SECRET_KEY", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StatusTTL != 15*time.Minute {
		t.Fatalf("unexpected default status TTL: %s", cfg.StatusTTL)
	}
}
