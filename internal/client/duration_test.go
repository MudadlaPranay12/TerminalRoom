package client

import (
	"testing"
	"time"
)

func TestParseDurationInputDefault(t *testing.T) {
	d, ok := parseDurationInput("")
	if !ok || d != 10*time.Minute {
		t.Errorf("empty should be 10m, got %v %v", d, ok)
	}
	d, ok = parseDurationInput("   ")
	if !ok || d != 10*time.Minute {
		t.Errorf("whitespace should be 10m")
	}
}

func TestParseDurationInputValid(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"1", 10 * time.Minute},
		{"2", 20 * time.Minute},
		{"3", 30 * time.Minute},
		{"4", 45 * time.Minute},
		{"5", 60 * time.Minute},
		{"10", 10 * time.Minute},
		{"20", 20 * time.Minute},
		{"30", 30 * time.Minute},
		{"45", 45 * time.Minute},
		{"60", 60 * time.Minute},
		{"10m", 10 * time.Minute},
		{"20m", 20 * time.Minute},
		{"30m", 30 * time.Minute},
		{"45m", 45 * time.Minute},
		{"60m", 60 * time.Minute},
		{"10 minutes", 10 * time.Minute},
		{"20 minutes", 20 * time.Minute},
		{"30 minutes", 30 * time.Minute},
		{"45 minutes", 45 * time.Minute},
		{"60 minutes", 60 * time.Minute},
		{" 30 ", 30 * time.Minute},
		{" 45M ", 45 * time.Minute},
	}
	for _, tt := range tests {
		d, ok := parseDurationInput(tt.input)
		if !ok || d != tt.want {
			t.Errorf("parseDurationInput(%q) = %v %v, want %v", tt.input, d, ok, tt.want)
		}
	}
}

func TestParseDurationInputInvalid(t *testing.T) {
	invalid := []string{"0", "-1", "15", "100", "abc", "6", "70", "30s", "10hour", "0m", "99"}
	for _, s := range invalid {
		if _, ok := parseDurationInput(s); ok {
			t.Errorf("parseDurationInput(%q) should be invalid", s)
		}
	}
}

func TestDurationConstants(t *testing.T) {
	if 10*time.Minute != 600*time.Second {
		t.Error("10m should be 600s")
	}
}
