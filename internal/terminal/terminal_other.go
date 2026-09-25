//go:build !windows

package terminal

// EnableVirtualTerminal is a no-op on non-Windows platforms (ANSI is native).
func EnableVirtualTerminal() error { return nil }

// RestoreVirtualTerminal is a no-op on non-Windows platforms.
func RestoreVirtualTerminal() {}

// IsVTEnabled reports whether VT is available. On non-Windows, ANSI is always supported.
func IsVTEnabled() bool { return true }
