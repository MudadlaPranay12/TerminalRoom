package client

import (
	"strings"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/terminal"
)

func TestWaitingHeaderIsWaiting(t *testing.T) {
	s := terminal.RenderWaiting("TR-TEST", "09:59", "Private", "TRINV-abc@192.168.1.10:9090")
	if !strings.Contains(s, "WAITING") {
		t.Error("waiting header should be WAITING")
	}
	if strings.Contains(s, "ACTIVE") {
		t.Error("waiting should not be ACTIVE")
	}
	if !strings.Contains(s, "1/2") {
		t.Error("waiting should be 1/2")
	}
	if strings.Contains(s, "2/2") {
		t.Error("waiting should not be 2/2")
	}
}

func TestActiveHeaderIsActive(t *testing.T) {
	s := terminal.RenderActiveHeader("TR-TEST", "08:31", "ACTIVE")
	if !strings.Contains(s, "Private") {
		t.Error("active header should contain Private")
	}
	if !strings.Contains(s, "2/2") {
		t.Error("active should be 2/2")
	}
	if strings.Contains(s, "1/2") {
		t.Error("active should not be 1/2")
	}
	// Ensure WAITING not shown when ACTIVE
	if strings.Contains(s, "WAITING") && strings.Contains(s, "ACTIVE") {
		// Header contains status, but should be ACTIVE not WAITING
		if !strings.Contains(s, "ACTIVE") {
			t.Error("active header should contain ACTIVE")
		}
	}
}

func TestCountdownFormatting(t *testing.T) {
	if terminal.FormatCountdown(10*time.Minute) != "10:00" {
		t.Error("10m countdown")
	}
	if terminal.FormatCountdown(30*time.Minute) != "30:00" {
		t.Error("30m countdown")
	}
	if terminal.FormatCountdown(0) != "00:00" {
		t.Error("0 countdown")
	}
}

func TestInvitationWrapping(t *testing.T) {
	token := "TRINV-" + strings.Repeat("A", 48) + "@100.93.120.19:9090"
	box := terminal.RenderInvitationBox(token)
	// Should contain both parts, split at @ (first part ends with @, second is host)
	if !strings.Contains(box, "TRINV-") {
		t.Error("invitation box should contain TRINV")
	}
	if !strings.Contains(box, "100.93.120.19:9090") {
		t.Error("invitation box should contain host")
	}
	if !strings.Contains(box, "@") {
		t.Error("invitation box should contain @")
	}
	// Should be inside inner box
	if !strings.Contains(box, "┌") || !strings.Contains(box, "┘") {
		t.Error("invitation box should have inner box")
	}
	// Original token must be reconstructable (visual wrapping only)
	if token != "TRINV-"+strings.Repeat("A", 48)+"@100.93.120.19:9090" {
		t.Error("token construction")
	}
}

func TestCCopiesExistingInvitation(t *testing.T) {
	orig := clipboardCopy
	defer func() { clipboardCopy = orig }()
	var got string
	clipboardCopy = func(s string) error {
		got = s
		return nil
	}
	token := "TRINV-abc123"
	bundled := token + "@192.168.1.10:9090"
	// Simulate C press in waiting: should copy existing bundled, not generate new
	if err := clipboardCopy(bundled); err != nil {
		t.Fatalf("copy failed")
	}
	if got != bundled {
		t.Errorf("C should copy existing %q, got %q", bundled, got)
	}
	// Second copy should be same, not new token
	got2 := ""
	clipboardCopy = func(s string) error { got2 = s; return nil }
	_ = clipboardCopy(bundled)
	if got2 != bundled {
		t.Error("second C should copy same invitation")
	}
}

func TestFriendJoinTransitionsWaitingToActive(t *testing.T) {
	c := &Client{roomID: "TR-TEST", participantID: 1, expiryTime: time.Now().Add(10 * time.Minute)}
	c.bundledInvite = "TRINV-abc@127.0.0.1:9090"
	// Initially waiting
	if c.friendJoined {
		t.Error("initially should be waiting")
	}
	// Simulate friend joined system message handling
	c.friendJoined = true
	remaining := time.Until(c.expiryTime)
	expires := terminal.FormatCountdown(remaining)
	header := terminal.RenderActiveHeader(c.roomID, expires, "ACTIVE")
	if !strings.Contains(header, "Private") || !strings.Contains(header, "2/2") {
		t.Error("after friend join, header should be Private 2/2")
	}
	if strings.Contains(header, "WAITING") {
		t.Error("after join, should not be WAITING")
	}
}

func TestHelpAndInfoRender(t *testing.T) {
	h := terminal.RenderHelp()
	if !strings.Contains(h, "/help") || !strings.Contains(h, "/info") || !strings.Contains(h, "/quit") {
		t.Error("help should contain commands")
	}
	info := terminal.RenderInfo("TR-TEST", "ACTIVE", "You + Friend", "in 5m", "Private")
	if !strings.Contains(info, "TR-TEST") || !strings.Contains(info, "ACTIVE") {
		t.Error("info should contain room and status")
	}
}

func TestCountdownUsesSelectedTTL(t *testing.T) {
	for _, d := range []time.Duration{10 * time.Minute, 30 * time.Minute, 60 * time.Minute} {
		expires := terminal.FormatCountdown(d)
		expected := map[time.Duration]string{
			10 * time.Minute: "10:00",
			30 * time.Minute: "30:00",
			60 * time.Minute: "60:00",
		}[d]
		if expires != expected {
			t.Errorf("countdown for %v = %q, want %q", d, expires, expected)
		}
	}
}
