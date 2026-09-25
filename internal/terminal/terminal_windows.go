//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	origStdoutMode  uint32
	origStderrMode  uint32
	stdoutVTEnabled bool
	stderrVTEnabled bool
)

// EnableVirtualTerminal enables ENABLE_VIRTUAL_TERMINAL_PROCESSING for the console output handles.
// It gracefully does nothing if output is redirected or the console does not support VT.
// It never returns an error that should cause application startup failure.
func EnableVirtualTerminal() error {
	// Stdout
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err == nil {
		origStdoutMode = mode
		newMode := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		if newMode == mode {
			stdoutVTEnabled = true
		} else if err := windows.SetConsoleMode(handle, newMode); err == nil {
			stdoutVTEnabled = true
		}
	}
	// Stderr
	handleErr := windows.Handle(os.Stderr.Fd())
	var modeErr uint32
	if err := windows.GetConsoleMode(handleErr, &modeErr); err == nil {
		origStderrMode = modeErr
		newMode := modeErr | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		if newMode == modeErr {
			stderrVTEnabled = true
		} else if err := windows.SetConsoleMode(handleErr, newMode); err == nil {
			stderrVTEnabled = true
		}
	}
	return nil
}

// RestoreVirtualTerminal restores the original console modes if they were changed.
// It is safe to call even if EnableVirtualTerminal was not called or failed.
func RestoreVirtualTerminal() {
	if stdoutVTEnabled {
		handle := windows.Handle(os.Stdout.Fd())
		_ = windows.SetConsoleMode(handle, origStdoutMode)
		stdoutVTEnabled = false
	}
	if stderrVTEnabled {
		handle := windows.Handle(os.Stderr.Fd())
		_ = windows.SetConsoleMode(handle, origStderrMode)
		stderrVTEnabled = false
	}
}

// IsVTEnabled reports whether virtual terminal processing is enabled for stdout.
func IsVTEnabled() bool { return stdoutVTEnabled }
