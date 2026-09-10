package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var ErrInvalidState = errors.New("invalid state")

type Status string

const (
	StatusAvailable Status = "available"
	StatusNeutral   Status = "neutral"
	StatusBusy      Status = "busy"
	StatusUnknown   Status = "unknown"
)

type Hand struct {
	Gesture    string  `json:"gesture"`
	Confidence float64 `json:"confidence"`
	Handedness string  `json:"handedness,omitempty"`
}

type Face struct {
	Expression string  `json:"expression"`
	Confidence float64 `json:"confidence"`
}

type Source string

const (
	SourceHand   Source = "hand"
	SourceFace   Source = "face"
	SourceNone   Source = "none"
	SourceManual Source = "manual"
)

type State struct {
	RoomID     string    `json:"roomId"`
	UserID     string    `json:"userId"`
	ClientID   string    `json:"clientId"`
	Sequence   uint64    `json:"sequence"`
	CapturedAt time.Time `json:"capturedAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Status     Status    `json:"status"`
	Hand       *Hand     `json:"hand,omitempty"`
	Face       *Face     `json:"face,omitempty"`
	Source     Source    `json:"source"`
}

func (s State) Validate(now time.Time) error {
	if err := s.validate(now); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	return nil
}

func (s State) validate(now time.Time) error {
	if s.Sequence == 0 {
		return errors.New("sequence must be positive")
	}
	if s.CapturedAt.IsZero() {
		return errors.New("capturedAt is required")
	}
	if s.CapturedAt.Before(now.Add(-5*time.Minute)) || s.CapturedAt.After(now.Add(30*time.Second)) {
		return errors.New("capturedAt is outside the accepted time window")
	}
	if s.ExpiresAt.IsZero() || !s.ExpiresAt.After(s.ReceivedAt) {
		return errors.New("expiresAt must be after receivedAt")
	}
	if s.Status != StatusAvailable && s.Status != StatusNeutral && s.Status != StatusBusy && s.Status != StatusUnknown {
		return errors.New("invalid status")
	}
	if s.Hand != nil {
		if !validConfidence(s.Hand.Confidence) {
			return errors.New("hand confidence must be between 0 and 1")
		}
		if s.Hand.Gesture != "thumb_up" && s.Hand.Gesture != "sideways_thumb" && s.Hand.Gesture != "thumb_down" && s.Hand.Gesture != "unknown" {
			return errors.New("invalid hand gesture")
		}
		if s.Hand.Handedness != "" && s.Hand.Handedness != "Left" && s.Hand.Handedness != "Right" {
			return errors.New("invalid handedness")
		}
	}
	if s.Face != nil {
		if !validConfidence(s.Face.Confidence) {
			return errors.New("face confidence must be between 0 and 1")
		}
		if s.Face.Expression != "smile" && s.Face.Expression != "frown" && s.Face.Expression != "surprised" && s.Face.Expression != "neutral" && s.Face.Expression != "unknown" {
			return errors.New("invalid face expression")
		}
	}
	if err := s.validateSource(); err != nil {
		return err
	}
	return nil
}

func validConfidence(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func (s State) validateSource() error {
	switch s.Source {
	case SourceHand:
		if s.Hand == nil || s.Hand.Gesture == "unknown" {
			return errors.New("hand source requires a recognized hand gesture")
		}
		expected := map[string]Status{"thumb_up": StatusAvailable, "sideways_thumb": StatusNeutral, "thumb_down": StatusBusy}[s.Hand.Gesture]
		if s.Status != expected {
			return fmt.Errorf("status does not match hand gesture %q", s.Hand.Gesture)
		}
	case SourceFace:
		if s.Face == nil || (s.Face.Expression != "smile" && s.Face.Expression != "frown") {
			return errors.New("face source requires an actionable expression")
		}
		expected := map[string]Status{"smile": StatusAvailable, "frown": StatusBusy}[s.Face.Expression]
		if s.Status != expected {
			return fmt.Errorf("status does not match face expression %q", s.Face.Expression)
		}
	case SourceNone:
		if s.Status != StatusUnknown {
			return errors.New("none source requires unknown status")
		}
	case SourceManual:
		if s.Status != StatusAvailable && s.Status != StatusNeutral && s.Status != StatusBusy {
			return errors.New("manual source requires an explicit status")
		}
		if s.Hand != nil || s.Face != nil {
			return errors.New("manual source must not include hand or face data")
		}
	default:
		return errors.New("invalid recognition source")
	}
	return nil
}
