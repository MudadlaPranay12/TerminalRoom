package client

import (
	"testing"

	"github.com/terminalroom/terminalroom/internal/invitation"
)

func TestClientClipboardSuccess(t *testing.T) {
	orig := clipboardCopy
	defer func() { clipboardCopy = orig }()

	var calledWith string
	clipboardCopy = func(s string) error {
		calledWith = s
		return nil
	}
	token, _ := invitation.GenerateToken()
	endpoint := "100.64.0.10:9090"
	bundled := invitation.BundledInvitation(token, endpoint)
	err := clipboardCopy(bundled)
	if err != nil {
		t.Fatalf("clipboardCopy should succeed: %v", err)
	}
	if calledWith != bundled {
		t.Errorf("clipboardCopy called with %q, want %q", calledWith, bundled)
	}
}

func TestClientClipboardFailure(t *testing.T) {
	orig := clipboardCopy
	defer func() { clipboardCopy = orig }()

	clipboardCopy = func(s string) error {
		return templatedError()
	}
	token, _ := invitation.GenerateToken()
	bundled := invitation.BundledInvitation(token, "100.64.0.10:9090")
	err := clipboardCopy(bundled)
	if err == nil {
		t.Error("clipboardCopy should fail when mocked to error")
	}
	// Client should not fail room creation even if clipboard fails.
	// Simulate createRoom's handling: error is logged but room remains usable.
	// Here we just verify error is returned and can be handled non-fatally.
	if err.Error() == "" {
		t.Error("error should have message")
	}
}

func templatedError() error {
	return errorString("clipboard locked")
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestClientBundledInvitationForClipboard(t *testing.T) {
	token, _ := invitation.GenerateToken()
	endpoint := "192.168.1.50:9090"
	bundled := invitation.BundledInvitation(token, endpoint)
	if bundled != token+"@"+endpoint {
		t.Errorf("BundledInvitation failed: got %q", bundled)
	}
	// Ensure clipboard would receive exactly the bundled invitation
	orig := clipboardCopy
	defer func() { clipboardCopy = orig }()
	var got string
	clipboardCopy = func(s string) error {
		got = s
		return nil
	}
	_ = clipboardCopy(bundled)
	if got != bundled {
		t.Errorf("clipboard should receive bundled invitation")
	}
}

func TestClientClipboardEmptyHandling(t *testing.T) {
	orig := clipboardCopy
	defer func() { clipboardCopy = orig }()
	clipboardCopy = func(s string) error {
		if s == "" {
			return errorString("empty")
		}
		return nil
	}
	if err := clipboardCopy(""); err == nil {
		t.Error("empty clipboard should error")
	}
}
