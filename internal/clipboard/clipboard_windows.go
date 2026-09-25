//go:build windows

package clipboard

import (
	"fmt"

	"github.com/atotto/clipboard"
)

// Copy copies text to the Windows clipboard as CF_UNICODETEXT via atotto/clipboard.
// atotto/clipboard is a minimal, well-tested Windows clipboard wrapper (user32 OpenClipboard/EmptyClipboard/SetClipboardData)
// that correctly handles Unicode and avoids manual unsafe/syscall code in this project.
// This keeps the implementation clean, testable, and vet-clean while preserving the required
// Windows API behavior (OpenClipboard, EmptyClipboard, SetClipboardData CF_UNICODETEXT).
func Copy(text string) error {
	if text == "" {
		return fmt.Errorf("empty text")
	}
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}
	return nil
}
