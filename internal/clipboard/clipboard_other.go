//go:build !windows

package clipboard

import "fmt"

// Copy on non-Windows returns an error indicating clipboard not supported.
// This allows the client to fallback to manual copy without failing the room.
func Copy(text string) error {
	if text == "" {
		return fmt.Errorf("empty text")
	}
	return fmt.Errorf("clipboard not supported on this platform")
}
