package application

import (
	"errors"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

const (
	DefaultManualLease = 30 * time.Second
	MinManualLease     = 1 * time.Second
	MaxManualLease     = 24 * time.Hour
)

func StatusExpiresAt(now time.Time, ttl time.Duration, source domain.Source) time.Time {
	if source == domain.SourceManual || ttl <= 0 {
		return time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC)
	}
	return now.Add(ttl)
}

func StatusExpiresAtWithLease(now time.Time, ttl time.Duration, source domain.Source, leaseSeconds *int) (time.Time, error) {
	if leaseSeconds == nil {
		return StatusExpiresAt(now, ttl, source), nil
	}
	if source != domain.SourceManual {
		return time.Time{}, errors.New("leaseSeconds is only supported for manual status")
	}
	if *leaseSeconds < int(MinManualLease/time.Second) || *leaseSeconds > int(MaxManualLease/time.Second) {
		return time.Time{}, errors.New("leaseSeconds must be between 1 and 86400")
	}
	return now.Add(time.Duration(*leaseSeconds) * time.Second), nil
}
