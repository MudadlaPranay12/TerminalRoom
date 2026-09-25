package room

import (
	"testing"
	"time"
)

func TestDefaultDurationIs10Minutes(t *testing.T) {
	if DefaultRoomTTL != 10*time.Minute {
		t.Errorf("DefaultRoomTTL = %v, want 10m", DefaultRoomTTL)
	}
	if MinRoomTTL != 10*time.Minute {
		t.Errorf("MinRoomTTL = %v, want 10m", MinRoomTTL)
	}
	if MaxRoomTTL != 60*time.Minute {
		t.Errorf("MaxRoomTTL = %v, want 60m", MaxRoomTTL)
	}
}

func TestAllowedDurations(t *testing.T) {
	expected := []time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute}
	if len(AllowedRoomTTLs) != len(expected) {
		t.Fatalf("AllowedRoomTTLs len %d, want %d", len(AllowedRoomTTLs), len(expected))
	}
	for i, d := range expected {
		if AllowedRoomTTLs[i] != d {
			t.Errorf("Allowed[%d] = %v, want %v", i, AllowedRoomTTLs[i], d)
		}
	}
}

func TestIsValidRoomTTL(t *testing.T) {
	valid := []time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute, 15 * time.Minute, 3600 * time.Second, 600 * time.Second}
	for _, d := range valid {
		if !IsValidRoomTTL(d) {
			t.Errorf("IsValidRoomTTL(%v) should be valid", d)
		}
	}
	invalid := []time.Duration{9 * time.Minute, 5 * time.Minute, 0, -1 * time.Minute, 61 * time.Minute, 100 * time.Minute, 599 * time.Second, 3601 * time.Second}
	for _, d := range invalid {
		if IsValidRoomTTL(d) {
			t.Errorf("IsValidRoomTTL(%v) should be invalid", d)
		}
	}
}

func TestNewRoomWithTTLReflectsDuration(t *testing.T) {
	for _, d := range AllowedRoomTTLs {
		r := NewRoomWithTTL("TR-TEST", d)
		remaining := r.TimeRemaining()
		// Allow small drift
		if remaining < d-time.Second || remaining > d+time.Second {
			t.Errorf("NewRoomWithTTL %v remaining %v", d, remaining)
		}
		// ExpiresAt should be CreatedAt + d
		diff := r.ExpiresAt.Sub(r.CreatedAt)
		if diff != d {
			t.Errorf("ExpiresAt diff %v, want %v", diff, d)
		}
	}
}

func TestCountdownUsesSelectedTTL(t *testing.T) {
	// Simulate that expiry is set from server's MsgExpiry which is room.ExpiresAt
	r := NewRoomWithTTL("TR-TEST", 30*time.Minute)
	// Client would receive expiryStr = r.ExpiresAt.Format(time.RFC3339) and set expiryTime
	// Countdown should start at ~30:00
	if r.TimeRemaining() < 29*time.Minute || r.TimeRemaining() > 30*time.Minute {
		t.Errorf("30m room should have ~30m remaining, got %v", r.TimeRemaining())
	}
}
