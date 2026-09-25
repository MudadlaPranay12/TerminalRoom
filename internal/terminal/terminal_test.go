package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "expired"},
		{-1 * time.Second, "expired"},
		{5 * time.Second, "5s"},
		{30 * time.Second, "30s"},
		{1 * time.Minute, "1m"},
		{1*time.Minute + 30*time.Second, "1m 30s"},
		{5 * time.Minute, "5m"},
		{9*time.Minute + 42*time.Second, "9m 42s"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 15*time.Minute, "1h 15m"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.input)
		if got != tt.expected {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatCountdown(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "00:00"},
		{-1 * time.Second, "00:00"},
		{5 * time.Second, "00:05"},
		{30 * time.Second, "00:30"},
		{1 * time.Minute, "01:00"},
		{5*time.Minute + 42*time.Second, "05:42"},
		{9*time.Minute + 9*time.Second, "09:09"},
		{59*time.Minute + 59*time.Second, "59:59"},
	}
	for _, tt := range tests {
		got := FormatCountdown(tt.input)
		if got != tt.expected {
			t.Errorf("FormatCountdown(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSessionStatusText(t *testing.T) {
	tests := []struct {
		isActive         bool
		state            string
		participantCount int
		expected         string
	}{
		{false, "waiting", 1, "WAITING"},
		{false, "waiting", 0, "WAITING"},
		{true, "active", 2, "ACTIVE"},
		{false, "active", 2, "ACTIVE"},
		{false, "expiring", 2, "EXPIRING"},
		{false, "closing", 0, "CLOSING"},
		{false, "destroyed", 0, "DESTROYED"},
	}
	for _, tt := range tests {
		got := SessionStatusText(tt.isActive, tt.state, tt.participantCount)
		if got != tt.expected {
			t.Errorf("SessionStatusText(%v, %q, %d) = %q, want %q",
				tt.isActive, tt.state, tt.participantCount, got, tt.expected)
		}
	}
}

func TestFormatTimestamp(t *testing.T) {
	ts := time.Date(2026, 9, 18, 17, 42, 3, 0, time.Local)
	got := FormatTimestamp(ts)
	if got != "17:42:03" {
		t.Errorf("FormatTimestamp = %q, want %q", got, "17:42:03")
	}
}

func TestFormatNowTimestamp(t *testing.T) {
	got := FormatNowTimestamp()
	if len(got) != 8 || got[2] != ':' || got[5] != ':' {
		t.Errorf("FormatNowTimestamp = %q, want HH:MM:SS format", got)
	}
}

func TestNetworkTypeFromEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		expected string
	}{
		{"localhost:9090", "Local"},
		{"127.0.0.1:9090", "Local"},
		{"192.168.1.100:9090", "Private"},
		{"10.0.0.1:9090", "Private"},
		{"172.16.0.1:9090", "Private"},
		{"100.64.0.1:9090", "Private"},
		{"8.8.8.8:9090", "Private"},
	}
	for _, tt := range tests {
		got := NetworkTypeFromEndpoint(tt.endpoint)
		if got != tt.expected {
			t.Errorf("NetworkTypeFromEndpoint(%q) = %q, want %q", tt.endpoint, got, tt.expected)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.0.1", false},
		{"192.168.1.1", true},
		{"192.168.0.1", true},
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.63.0.1", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"not-an-ip", false},
	}
	for _, tt := range tests {
		got := isPrivateIP(tt.host)
		if got != tt.expected {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.host, got, tt.expected)
		}
	}
}

func TestBanner(t *testing.T) {
	b := Banner()
	if !strings.Contains(b, "=") {
		t.Errorf("Banner() should contain '=' chars, got %q", b)
	}
}

func TestSeparator(t *testing.T) {
	s := Separator()
	if !strings.Contains(s, "─") {
		t.Errorf("Separator() should contain box-drawing chars, got %q", s)
	}
}

func TestBoxLines(t *testing.T) {
	box := BoxLines([]string{"Hello", "World"})
	if !strings.Contains(box, "Hello") {
		t.Error("BoxLines should contain 'Hello'")
	}
	if !strings.Contains(box, "World") {
		t.Error("BoxLines should contain 'World'")
	}
	if !strings.HasPrefix(box, "+") {
		t.Error("BoxLines should start with '+'")
	}
	if !strings.HasSuffix(box, "+") {
		t.Error("BoxLines should end with '+'")
	}
}

func TestClearLine(t *testing.T) {
	cl := ClearLine()
	if !strings.HasPrefix(cl, "\r") {
		t.Errorf("ClearLine() should start with \\r, got %q", cl)
	}
	if IsVTEnabled() && !strings.Contains(cl, "\033[K") {
		t.Errorf("ClearLine() with VT should contain ANSI clear escape, got %q", cl)
	}
}

func TestRenderHome(t *testing.T) {
	home := RenderHome()
	if !strings.Contains(strings.ToUpper(home), "TERMINALROOM") {
		t.Error("RenderHome should contain TERMINALROOM")
	}
	if !strings.Contains(home, "PRIVATE") {
		t.Error("RenderHome should contain PRIVATE")
	}
	if !strings.Contains(home, "Create private room") {
		t.Error("RenderHome should contain Create private room")
	}
	if !strings.Contains(home, "Terminal ready") {
		t.Error("RenderHome should contain Terminal ready")
	}
	if !strings.Contains(home, "─") {
		t.Error("RenderHome should use thin separator")
	}
	if !strings.Contains(home, "╭") || !strings.Contains(home, "╰") {
		t.Error("RenderHome should be inside a fitted box with ╭/╰")
	}
	lines := strings.Split(strings.TrimSpace(home), "\n")
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Error("Home top border must be ╭─╮")
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
		t.Error("Home bottom border must be ╰─╯")
	}
	// Ensure menu inside box
	if !strings.Contains(home, "[J] Join private room") || !strings.Contains(home, "[Q] Quit") {
		t.Error("Home menu must be inside box")
	}
}

func TestRenderCreateRoom(t *testing.T) {
	box := RenderCreateRoom("TR-ABCDEF", "09:42", "Private")
	if !strings.Contains(box, "TR-ABCDEF") {
		t.Error("RenderCreateRoom should contain roomID")
	}
	if !strings.Contains(box, "PRIVATE ROOM") {
		t.Error("RenderCreateRoom should contain PRIVATE ROOM")
	}
	if !strings.Contains(box, "WAITING") {
		t.Error("RenderCreateRoom should contain WAITING")
	}
	if !strings.Contains(box, "09:42") {
		t.Error("RenderCreateRoom should contain expires")
	}
}

func TestRenderInvitationBox(t *testing.T) {
	token := "TRINV-abc123@192.168.1.10:9090"
	box := RenderInvitationBox(token)
	if !strings.Contains(box, "TRINV-abc123") {
		t.Error("RenderInvitationBox should contain token")
	}
	if !strings.Contains(box, "┌") || !strings.Contains(box, "┘") {
		t.Error("RenderInvitationBox should use inner box")
	}
}

func TestRenderActiveHeader(t *testing.T) {
	h := RenderActiveHeader("TR-123456", "08:31", "ACTIVE")
	if !strings.Contains(h, "TR-123456") {
		t.Error("RenderActiveHeader should contain roomID")
	}
	if !strings.Contains(h, "Private") {
		t.Error("RenderActiveHeader should contain Private")
	}
	if !strings.Contains(h, "08:31") {
		t.Error("RenderActiveHeader should contain remaining")
	}
	if !strings.Contains(h, "2/2") {
		t.Error("RenderActiveHeader should contain 2/2")
	}
}

func TestRenderDestroyed(t *testing.T) {
	d := RenderDestroyed()
	if !strings.Contains(d, "ROOM DESTROYED") {
		t.Error("RenderDestroyed should contain ROOM DESTROYED")
	}
	if !strings.Contains(d, "No chat history") {
		t.Error("RenderDestroyed should contain no history text")
	}
}

func TestRenderError(t *testing.T) {
	e := RenderError("ERROR", "Something went wrong")
	if !strings.Contains(e, "ERROR") || !strings.Contains(e, "Something went wrong") {
		t.Error("RenderError should contain title and reason")
	}
}

func TestRenderHelp(t *testing.T) {
	h := RenderHelp()
	if !strings.Contains(h, "/help") || !strings.Contains(h, "/info") || !strings.Contains(h, "/quit") {
		t.Error("RenderHelp should contain commands")
	}
}

func TestRenderInfo(t *testing.T) {
	info := RenderInfo("TR-123", "ACTIVE", "You + Friend", "in 5m", "Private")
	if !strings.Contains(info, "TR-123") || !strings.Contains(info, "ACTIVE") || !strings.Contains(info, "Private") {
		t.Error("RenderInfo should contain fields")
	}
}

func TestStatusDot(t *testing.T) {
	if !strings.Contains(StatusDot("ACTIVE"), "●") {
		t.Error("StatusDot ACTIVE should contain ●")
	}
	if !strings.Contains(StatusDot("WAITING"), "○") {
		t.Error("StatusDot WAITING should contain ○")
	}
	if StatusDot("EXPIRING") == "" {
		t.Error("StatusDot EXPIRING should not be empty")
	}
	// Restrained colors: should contain Private green when VT is enabled, but still contain the dot
	if !strings.Contains(RenderActiveHeader("TR-TEST", "08:31", "ACTIVE"), "Private") {
		t.Error("RenderActiveHeader should contain Private")
	}
}
