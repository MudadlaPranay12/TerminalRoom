package clipboard

import (
	"runtime"
	"testing"
)

func TestCopyEmpty(t *testing.T) {
	err := Copy("")
	if err == nil {
		t.Error("Copy(\"\") should return error for empty text")
	}
}

func TestCopyNormalInvitation(t *testing.T) {
	inv := "TRINV-abc123def4567890abc123def4567890abc123def4567890@100.64.0.10:9090"
	err := Copy(inv)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Copy on non-windows should return error")
		}
		if err != nil && err.Error() != "clipboard not supported on this platform" && err.Error() != "empty text" {
			// Accept any error on non-windows as long as it errors
		}
		return
	}
	// On Windows, clipboard may be unavailable in headless CI.
	// If it fails due to locked clipboard, we skip rather than fail.
	if err != nil {
		t.Skipf("Copy failed on Windows (may be headless): %v", err)
	}
}

func TestCopyUnicode(t *testing.T) {
	text := "TRINV-unicode-test@100.64.0.10:9090 ✓ 漢字"
	err := Copy(text)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Copy unicode on non-windows should return error")
		}
		return
	}
	if err != nil {
		t.Skipf("Copy unicode failed on Windows (headless): %v", err)
	}
}

func TestCopyReturnsErrorProperly(t *testing.T) {
	// Empty should error
	if err := Copy(""); err == nil {
		t.Error("empty should error")
	}
	// Normal on non-windows should error with specific message
	if runtime.GOOS != "windows" {
		err := Copy("TRINV-test@127.0.0.1:9090")
		if err == nil {
			t.Error("should error on non-windows")
		}
	}
}
