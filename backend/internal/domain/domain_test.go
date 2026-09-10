package domain

import (
	"testing"
	"time"
)

func TestStateValidate(t *testing.T) {
	now := time.Now().UTC()
	valid := State{Sequence: 1, CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(15 * time.Minute), Status: StatusAvailable, Source: SourceHand, Hand: &Hand{Gesture: "thumb_up", Confidence: .9}}
	if err := valid.Validate(now); err != nil {
		t.Fatalf("valid state rejected: %v", err)
	}

	tests := []State{
		{CapturedAt: now, Status: StatusUnknown, Source: SourceNone},
		{Sequence: 1, CapturedAt: now.Add(-6 * time.Minute), Status: StatusUnknown, Source: SourceNone},
		{Sequence: 1, CapturedAt: now, Status: "invalid", Source: SourceNone},
		{Sequence: 1, CapturedAt: now, Status: StatusAvailable, Source: SourceHand, Hand: &Hand{Gesture: "thumb_up", Confidence: 1.1}},
		{Sequence: 1, CapturedAt: now, Status: StatusBusy, Source: SourceHand, Hand: &Hand{Gesture: "thumb_up", Confidence: .9}},
		{Sequence: 1, CapturedAt: now, Status: StatusNeutral, Source: SourceHand, Hand: &Hand{Gesture: "shaka", Confidence: .9}},
	}
	for i, state := range tests {
		if err := state.Validate(now); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestFaceDerivedStateValidate(t *testing.T) {
	now := time.Now().UTC()
	state := State{Sequence: 1, CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(15 * time.Minute), Status: StatusAvailable, Source: SourceFace,
		Face: &Face{Expression: "smile", Confidence: .8}}
	if err := state.Validate(now); err != nil {
		t.Fatalf("valid face state rejected: %v", err)
	}
	state.Status = StatusBusy
	if err := state.Validate(now); err == nil {
		t.Fatal("mismatched face status should fail")
	}
}

func TestManualSourceValidate(t *testing.T) {
	now := time.Now().UTC()
	state := State{Sequence: 1, CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(15 * time.Minute), Status: StatusBusy, Source: SourceManual}
	if err := state.Validate(now); err != nil {
		t.Fatalf("valid manual state rejected: %v", err)
	}

	state.Status = StatusUnknown
	if err := state.Validate(now); err == nil {
		t.Fatal("manual source with unknown status should fail")
	}

	state.Status = StatusBusy
	state.Hand = &Hand{Gesture: "thumb_down", Confidence: .9}
	if err := state.Validate(now); err == nil {
		t.Fatal("manual source with hand data should fail")
	}
}
