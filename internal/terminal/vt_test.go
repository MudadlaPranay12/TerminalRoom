package terminal

import (
	"testing"
)

func TestEnableVirtualTerminalDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EnableVirtualTerminal panicked: %v", r)
		}
	}()
	if err := EnableVirtualTerminal(); err != nil {
		t.Fatalf("EnableVirtualTerminal returned error: %v", err)
	}
	// Restore should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RestoreVirtualTerminal panicked: %v", r)
		}
	}()
	RestoreVirtualTerminal()
	// Second cycle should be safe
	if err := EnableVirtualTerminal(); err != nil {
		t.Fatalf("second Enable failed: %v", err)
	}
	RestoreVirtualTerminal()
	RestoreVirtualTerminal() // idempotent
}

func TestClearLineUsesVT(t *testing.T) {
	// ClearLine should return a string starting with \r
	s := ClearLine()
	if len(s) == 0 || s[0] != '\r' {
		t.Errorf("ClearLine should start with \\r, got %q", s)
	}
	// On non-windows, IsVTEnabled is true, so should contain \033[K
	// On windows, it depends on console; but should at least start with \r
}

func TestTerminalFormattingUnchanged(t *testing.T) {
	// Ensure existing formatting helpers still work
	if FormatDuration(0) != "expired" {
		t.Error("FormatDuration unchanged check failed")
	}
	if FormatCountdown(0) != "00:00" {
		t.Error("FormatCountdown unchanged")
	}
	if Banner() == "" {
		t.Error("Banner should not be empty")
	}
	if Separator() == "" {
		t.Error("Separator should not be empty")
	}
	if got := ClearLine(); got == "" {
		t.Error("ClearLine should not be empty")
	}
}

func TestIsVTEnabled(t *testing.T) {
	// Should not panic
	_ = IsVTEnabled()
	EnableVirtualTerminal()
	_ = IsVTEnabled()
	RestoreVirtualTerminal()
	_ = IsVTEnabled()
}
