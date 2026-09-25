package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDurationSelector(t *testing.T) {
	for _, d := range []time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute} {
		s := RenderDurationSelector(d)
		if !strings.Contains(s, "Create Private Room") {
			t.Error("selector should contain Create Private Room")
		}
		if !strings.Contains(s, FormatMinutes(d)) {
			t.Errorf("selector for %v should highlight %s", d, FormatMinutes(d))
		}
		if !strings.Contains(s, "●") {
			t.Error("selector should contain ● for selected")
		}
		if !strings.Contains(s, "10 minutes") || !strings.Contains(s, "60 minutes") {
			t.Error("selector should list all durations")
		}
	}
}

func TestFormatMinutes(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Minute, "10 minutes"},
		{20 * time.Minute, "20 minutes"},
		{30 * time.Minute, "30 minutes"},
		{45 * time.Minute, "45 minutes"},
		{60 * time.Minute, "60 minutes"},
	}
	for _, tt := range tests {
		if got := FormatMinutes(tt.d); got != tt.want {
			t.Errorf("FormatMinutes(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatDurationLabel(t *testing.T) {
	if FormatDurationLabel(10*time.Minute) != "10 minutes" {
		t.Error("FormatDurationLabel")
	}
}

func TestRenderCreateRoomShowsActualDuration(t *testing.T) {
	// Ensure that RenderCreateRoom uses the actual expires string, not hardcoded 10m
	s := RenderCreateRoom("TR-TEST", "30:00", "Private")
	if !strings.Contains(s, "30:00") {
		t.Error("RenderCreateRoom should show actual expires, not hardcoded 10m")
	}
	if strings.Contains(s, "in 10 minutes") && !strings.Contains(s, "30:00") {
		t.Error("should not show 10m when 30m selected")
	}
}
