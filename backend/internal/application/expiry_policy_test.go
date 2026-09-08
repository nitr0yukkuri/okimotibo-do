package application

import (
	"testing"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

func TestStatusExpiryPolicySeparatesAutomaticTTLAndManualLease(t *testing.T) {
	now := time.Now().UTC()
	if !StatusExpiresAt(now, 15*time.Minute, domain.SourceManual).After(now.Add(24 * time.Hour)) {
		t.Fatal("manual status should be held until an explicit clear")
	}
	if got := StatusExpiresAt(now, 15*time.Minute, domain.SourceHand); !got.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("automatic status expiry = %v", got)
	}
	lease := 10
	got, err := StatusExpiresAtWithLease(now, 15*time.Minute, domain.SourceManual, &lease)
	if err != nil || !got.Equal(now.Add(10*time.Second)) {
		t.Fatalf("manual lease expiry = %v, err=%v", got, err)
	}
}
