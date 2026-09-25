package terminal

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var mu sync.Mutex

func Printf(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(os.Stdout, format, args...)
}

func Print(format string, args ...interface{}) {
	Printf(format, args...)
}

func Println(s string) {
	Printf("%s\n", s)
}

func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	total := int(d.Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60

	if h > 0 {
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		if s > 0 {
			return fmt.Sprintf("%dm %ds", m, s)
		}
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", s)
}

func FormatCountdown(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}
	total := int(d.Seconds())
	m := total / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func SessionStatusText(isActive bool, state string, participantCount int) string {
	switch state {
	case "expiring":
		return "EXPIRING"
	case "closing":
		return "CLOSING"
	case "destroyed":
		return "DESTROYED"
	default:
		if participantCount >= 2 || isActive {
			return "ACTIVE"
		}
		return "WAITING"
	}
}

func FormatTimestamp(t time.Time) string {
	return t.Format("15:04:05")
}

func FormatNowTimestamp() string {
	return FormatTimestamp(time.Now())
}

func NetworkTypeFromEndpoint(endpoint string) string {
	host, _, _ := strings.Cut(endpoint, ":")
	if host == "" {
		host = endpoint
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "Local"
	}
	if isPrivateIP(host) {
		return "Private"
	}
	return "Private"
}

func isPrivateIP(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	if parts[0] == "10" {
		return true
	}
	if parts[0] == "172" {
		n := 0
		fmt.Sscanf(parts[1], "%d", &n)
		if n >= 16 && n <= 31 {
			return true
		}
	}
	if parts[0] == "192" && parts[1] == "168" {
		return true
	}
	if parts[0] == "100" {
		n := 0
		fmt.Sscanf(parts[1], "%d", &n)
		if n >= 64 && n <= 127 {
			return true
		}
	}
	return false
}

func Banner() string {
	return "================================================"
}

func Header(title string) string {
	return title
}

func BoxLines(lines []string) string {
	// Legacy helper preserved for compatibility; now uses visibleLen and new borders for consistency
	width := 46
	for _, l := range lines {
		clean := stripANSI(l)
		if w := visibleLen(clean); w+6 > width {
			width = w + 6
		}
	}
	var sb strings.Builder
	sb.WriteString("+" + strings.Repeat("-", width-2) + "+\n")
	for _, l := range lines {
		clean := stripANSI(l)
		padding := width - 4 - visibleLen(clean)
		if padding < 1 {
			padding = 1
		}
		sb.WriteString("| " + l + strings.Repeat(" ", padding) + "|\n")
	}
	sb.WriteString("+" + strings.Repeat("-", width-2) + "+")
	return sb.String()
}

func Separator() string {
	return "────────────────────────────────────────────────"
}

func ClearLine() string {
	if IsVTEnabled() {
		return "\r\033[K"
	}
	return "\r"
}

func colorGreen(s string) string {
	if IsVTEnabled() {
		return "\033[32m" + s + "\033[0m"
	}
	return s
}

func colorYellow(s string) string {
	if IsVTEnabled() {
		return "\033[33m" + s + "\033[0m"
	}
	return s
}

func colorRed(s string) string {
	if IsVTEnabled() {
		return "\033[31m" + s + "\033[0m"
	}
	return s
}

func colorMuted(s string) string {
	if IsVTEnabled() {
		return "\033[90m" + s + "\033[0m"
	}
	return s
}

// ── Polished UI helpers (F-UI) ────────────────────────────────────────────

func StatusDot(state string) string {
	switch state {
	case "ACTIVE", "active":
		return colorGreen("●")
	case "WAITING", "waiting":
		return colorMuted("○")
	case "EXPIRING", "expiring":
		return colorYellow("◐")
	case "CLOSING", "closing":
		return colorYellow("◑")
	case "DESTROYED", "destroyed":
		return colorMuted("○")
	default:
		return colorMuted("○")
	}
}

func stripANSI(s string) string {
	for {
		start := strings.Index(s, "\033[")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "m")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	return s
}

func visibleLen(s string) int {
	clean := stripANSI(s)
	count := 0
	for range clean {
		count++
	}
	return count
}

func centerText(s string, width int) string {
	vis := visibleLen(s)
	if vis >= width {
		runes := []rune(stripANSI(s))
		if len(runes) > width {
			runes = runes[:width]
		}
		return string(runes)
	}
	left := (width - vis) / 2
	right := width - vis - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func padLine(s string, width int) string {
	vis := visibleLen(s)
	if vis >= width {
		clean := stripANSI(s)
		runes := []rune(clean)
		if len(runes) > width {
			return string(runes[:width])
		}
		return clean
	}
	return s + strings.Repeat(" ", width-vis)
}

func padField(label, value string, width int) string {
	line := fmt.Sprintf("  %-10s %s", label, value)
	vis := visibleLen(line)
	if vis >= width {
		return line
	}
	return line + strings.Repeat(" ", width-vis)
}

func getTerminalWidth() int {
	if colStr := os.Getenv("COLUMNS"); colStr != "" {
		var n int
		if _, err := fmt.Sscanf(colStr, "%d", &n); err == nil && n > 0 {
			// Clamp to sensible range 50..80 then subtract margin
			if n > 80 {
				n = 80
			}
			if n < 50 {
				n = 50
			}
			// inner width is n-2 for borders? Use n-4 for margin then inner will be n-4
			// Return inner width (without borders) approximation
			// For consistency with existing code: return n-4
			if n-4 < 50 {
				return 50 - 2 // but keep at least 46
			}
			return n - 4
		}
	}
	return 60
}

func panelInnerWidth() int {
	w := getTerminalWidth()
	if w < 50 {
		w = 50
	}
	if w > 68 {
		w = 68
	}
	return w
}

func renderPanel(title string, lines []string) string {
	maxVis := 0
	if title != "" {
		if l := visibleLen(title); l > maxVis {
			maxVis = l
		}
	}
	for _, l := range lines {
		if vis := visibleLen(l); vis > maxVis {
			maxVis = vis
		}
	}
	termW := getTerminalWidth()
	inner := maxVis + 4
	if inner < 40 {
		inner = 40
	}
	if inner > termW {
		inner = termW
	}
	if inner < 50 {
		inner = 50
	}
	if inner > termW {
		inner = termW
	}
	// Also cap at 68 for normal terminals
	if inner > 68 {
		inner = 68
	}
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	if title != "" {
		b.WriteString("│" + padLine("  "+title, inner) + "│\n")
		b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	}
	for _, raw := range lines {
		if raw == "" {
			b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
			continue
		}
		if raw == "---" {
			b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
			continue
		}
		wrapped := wrapText(raw, inner-4)
		for _, wline := range wrapped {
			b.WriteString("│  " + padLine(wline, inner-4) + "  │\n")
		}
	}
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderHome returns the boxed home screen — entire content inside one fitted panel.
func RenderHome() string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText("TerminalRoom", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText(colorMuted("Terminal ready."), inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText("Create a private room for", inner) + "│\n")
	b.WriteString("│" + centerText("exactly two people.", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	// Separator inside box
	b.WriteString("│" + centerText(strings.Repeat("─", inner-4), inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│  " + padLine("> [C] Create private room", inner-4) + "  │\n")
	b.WriteString("│  " + padLine("> [J] Join private room", inner-4) + "  │\n")
	b.WriteString("│  " + padLine("> [H] Help", inner-4) + "  │\n")
	b.WriteString("│  " + padLine("> [D] Diagnose", inner-4) + "  │\n")
	b.WriteString("│  " + padLine("> [Q] Quit", inner-4) + "  │\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText(colorMuted("PRIVATE \u2022 TEMPORARY \u2022 TWO-PERSON"), inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderCreateRoom returns a clean private room box — selective.
func RenderCreateRoom(roomID, expires, network string) string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + padLine("  PRIVATE ROOM", inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + padField("Room", roomID, inner) + "│\n")
	b.WriteString("│" + padField("Status", StatusDot("WAITING")+" WAITING FOR FRIEND", inner) + "│\n")
	b.WriteString("│" + padField("Expires", expires, inner) + "│\n")
	if network != "" {
		b.WriteString("│" + padField("Network", network, inner) + "│\n")
	}
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + padLine("  INVITATION", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderInvitationBox renders the invitation token in an inner box.
// Wraps cleanly at "@" so TRINV-... and @host:port appear on separate lines if needed.
func RenderInvitationBox(token string) string {
	innerW := 56
	// Adapt innerW to panel width if narrow
	term := panelInnerWidth()
	if term < 60 {
		innerW = term - 6
		if innerW < 30 {
			innerW = 30
		}
	}
	var b strings.Builder
	b.WriteString("  ┌" + strings.Repeat("─", innerW) + "┐\n")
	if idx := strings.Index(token, "@"); idx != -1 && visibleLen(token) > innerW {
		first := token[:idx+1]
		second := token[idx+1:]
		for len(first) > 0 {
			chunk := first
			if visibleLen(chunk) > innerW-1 {
				cut := cutVisible(first, innerW-1)
				chunk = first[:cut]
				first = first[cut:]
			} else {
				first = ""
			}
			b.WriteString("  │ " + padLine(chunk, innerW-1) + "│\n")
			if first == "" {
				break
			}
		}
		for len(second) > 0 {
			chunk := second
			if visibleLen(chunk) > innerW-1 {
				cut := cutVisible(second, innerW-1)
				chunk = second[:cut]
				second = second[cut:]
			} else {
				second = ""
			}
			b.WriteString("  │ " + padLine(chunk, innerW-1) + "│\n")
			if second == "" {
				break
			}
		}
	} else {
		for len(token) > 0 {
			chunk := token
			if visibleLen(chunk) > innerW-1 {
				cut := cutVisible(token, innerW-1)
				chunk = token[:cut]
				token = token[cut:]
			} else {
				token = ""
			}
			b.WriteString("  │ " + padLine(chunk, innerW-1) + "│\n")
			if token == "" {
				break
			}
		}
	}
	b.WriteString("  └" + strings.Repeat("─", innerW) + "┘")
	return b.String()
}

// cutVisible returns byte index where visible length == width, handling UTF-8
func cutVisible(s string, width int) int {
	vis := 0
	byteIdx := 0
	for byteIdx < len(s) {
		if strings.HasPrefix(s[byteIdx:], "\033[") {
			end := strings.Index(s[byteIdx:], "m")
			if end == -1 {
				break
			}
			byteIdx += end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(s[byteIdx:])
		if r == utf8.RuneError {
			byteIdx++
			vis++
		} else {
			byteIdx += size
			vis++
		}
		if vis >= width {
			break
		}
	}
	return byteIdx
}

// RenderActiveHeader returns a clean header box — selective structure.
func RenderActiveHeader(roomID, remaining, status string) string {
	inner := panelInnerWidth()
	count := "1/2"
	if status == "ACTIVE" || status == "active" {
		count = "2/2"
	}
	privateLabel := colorGreen("● Private")
	if status == "WAITING" || status == "waiting" {
		privateLabel = colorMuted("○ Waiting")
	}
	right := fmt.Sprintf("%s   %s   %s", privateLabel, colorMuted(count), colorMuted(remaining))
	if remaining == "" {
		right = fmt.Sprintf("%s   %s", privateLabel, colorMuted(count))
	}
	left := " TerminalRoom"
	visLeft := visibleLen(left)
	visRight := visibleLen(right)
	padding := inner - visLeft - visRight
	if padding < 1 {
		padding = 1
	}
	line1 := left + strings.Repeat(" ", padding) + right
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + padLine(line1, inner) + "│\n")
	b.WriteString("│" + padLine(colorMuted("  "+roomID), inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤")
	return b.String()
}

// RenderChatFooter returns the input prompt footer for active chat.
func RenderChatFooter() string {
	inner := panelInnerWidth()
	return "├" + strings.Repeat("─", inner) + "┤\n" + "│  > " + strings.Repeat(" ", inner-4) + "│\n" + "╰" + strings.Repeat("─", inner) + "╯"
}

// RenderDestroyed returns a clean destroyed box.
func RenderDestroyed() string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText("ROOM DESTROYED", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText("This temporary room has ended.", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText("No chat history was saved.", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + centerText(colorMuted("[Enter] Exit"), inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderError returns a clean error box — subtle red title, minimal.
func RenderError(title, reason string) string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + centerText(colorRed(title), inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	for _, line := range wrapText(reason, inner-4) {
		b.WriteString("│  " + padLine(line, inner-4) + "  │\n")
	}
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

func wrapText(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	if visibleLen(s) <= width {
		return []string{s}
	}
	var out []string
	remaining := strings.TrimSpace(s)
	for visibleLen(remaining) > width {
		// Prefer break at space
		// Find last space within width visible chars
		// We need to map visible position to byte index
		visCount := 0
		lastSpaceByte := -1
		byteIdx := 0
		for byteIdx < len(remaining) && visCount < width {
			if strings.HasPrefix(remaining[byteIdx:], "\033[") {
				end := strings.Index(remaining[byteIdx:], "m")
				if end == -1 {
					break
				}
				byteIdx += end + 1
				continue
			}
			r, size := utf8.DecodeRuneInString(remaining[byteIdx:])
			if r == ' ' {
				lastSpaceByte = byteIdx
			}
			if r != utf8.RuneError {
				byteIdx += size
			} else {
				byteIdx++
			}
			visCount++
		}
		cutByte := byteIdx
		if lastSpaceByte != -1 && lastSpaceByte > 0 {
			// Use space break if within second half
			spaceVis := visibleLen(remaining[:lastSpaceByte])
			if spaceVis > width/2 {
				cutByte = lastSpaceByte
			}
		}
		chunk := strings.TrimSpace(remaining[:cutByte])
		if chunk != "" {
			out = append(out, chunk)
		}
		if cutByte >= len(remaining) {
			remaining = ""
			break
		}
		remaining = strings.TrimSpace(remaining[cutByte:])
		if remaining == "" {
			break
		}
	}
	if remaining != "" {
		out = append(out, remaining)
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// RenderHelp returns a clean help box — selective.
func RenderHelp() string {
	inner := panelInnerWidth()
	// Use 48 inner for help to stay compact but still fitted; if narrow, use panel width
	if inner > 48 {
		inner = 48
		if getTerminalWidth() > 48 {
			// keep compact but not too narrow
		}
	}
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + centerText("HELP", inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + padLine("  /help    Show this help", inner) + "│\n")
	b.WriteString("│" + padLine("  /info    Show room information", inner) + "│\n")
	b.WriteString("│" + padLine("  /quit    Leave the room", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderInfo returns a clean info box — selective.
func RenderInfo(roomID, status, participant, expires, network string) string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + padLine("  Room Information", inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + padField("Room", roomID, inner) + "│\n")
	b.WriteString("│" + padField("Status", status, inner) + "│\n")
	b.WriteString("│" + padField("Participant", participant, inner) + "│\n")
	b.WriteString("│" + padField("Expires", expires, inner) + "│\n")
	b.WriteString("│" + padField("Network", network, inner) + "│\n")
	b.WriteString("│" + padField("Transport", "TLS", inner) + "│\n")
	b.WriteString("│" + padField("Persistence", "None", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderWaiting returns a single clean waiting box — selective structured box.
func RenderWaiting(roomID, expires, network, token string) string {
	inner := panelInnerWidth()
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	b.WriteString("│" + padLine("  "+colorMuted(StatusDot("WAITING")+" WAITING   1/2   "+expires), inner) + "│\n")
	b.WriteString("│" + padLine("  "+colorMuted(roomID), inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + padLine("  Waiting for friend...", inner) + "│\n")
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("│" + padLine("  INVITATION", inner) + "│\n")
	if token != "" {
		innerW := 56
		if inner < 60 {
			innerW = inner - 6
			if innerW < 30 {
				innerW = 30
			}
		}
		b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
		b.WriteString("│  ┌" + strings.Repeat("─", innerW) + "┐" + strings.Repeat(" ", inner-(2+innerW+2)) + "│\n")
		if idx := strings.Index(token, "@"); idx != -1 {
			first := token[:idx+1]
			second := token[idx+1:]
			// first part already includes @
			b.WriteString("│  │ " + padLine(first, innerW-1) + "│" + strings.Repeat(" ", inner-(2+innerW+2)) + "│\n")
			for len(second) > 0 {
				chunk := second
				if visibleLen(chunk) > innerW-1 {
					cut := cutVisible(second, innerW-1)
					chunk = second[:cut]
					second = second[cut:]
				} else {
					second = ""
				}
				b.WriteString("│  │ " + padLine(chunk, innerW-1) + "│" + strings.Repeat(" ", inner-(2+innerW+2)) + "│\n")
				if second == "" {
					break
				}
			}
		} else {
			remaining := token
			for len(remaining) > 0 {
				chunk := remaining
				if visibleLen(chunk) > innerW-1 {
					cut := cutVisible(remaining, innerW-1)
					chunk = remaining[:cut]
					remaining = remaining[cut:]
				} else {
					remaining = ""
				}
				b.WriteString("│  │ " + padLine(chunk, innerW-1) + "│" + strings.Repeat(" ", inner-(2+innerW+2)) + "│\n")
				if remaining == "" {
					break
				}
			}
		}
		b.WriteString("│  └" + strings.Repeat("─", innerW) + "┘" + strings.Repeat(" ", inner-(2+innerW+2)) + "│\n")
		b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
		b.WriteString("│" + padLine("  "+colorMuted("Invitation copied to clipboard."), inner) + "│\n")
	}
	b.WriteString("│" + strings.Repeat(" ", inner) + "│\n")
	b.WriteString("├" + strings.Repeat("─", inner) + "┤\n")
	b.WriteString("│" + padLine("  [C] Copy invitation    [H] Help    [Q] Quit", inner) + "│\n")
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// RenderStatusLine returns a single stable status line for the header/waiting bar.
func RenderStatusLine(status, count, remaining string) string {
	dot := StatusDot(status)
	return fmt.Sprintf("%s %s  %s  %s", dot, status, colorMuted(count), colorMuted(remaining))
}

// UpdateHeaderInPlace redraws the header's first line in place at the top.
func UpdateHeaderInPlace(roomID, remaining, status string) {
	header := RenderActiveHeader(roomID, remaining, status)
	lines := strings.Split(header, "\n")
	if len(lines) < 2 {
		return
	}
	firstContent := lines[1]
	Printf("\033[s\033[H\033[2B\r\033[K%s\033[u", firstContent)
}

// RenderDurationSelector returns a fitted panel for duration selection.
func RenderDurationSelector(selected time.Duration) string {
	lines := []string{
		"",
		"How long should this room exist?",
		"",
	}
	for _, d := range []time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute} {
		marker := " "
		if d == selected {
			marker = colorGreen("●")
		}
		lines = append(lines, fmt.Sprintf("  %s %s", marker, FormatMinutes(d)))
	}
	lines = append(lines, "", colorMuted("Enter to select."))
	return renderPanel("Create Private Room", lines)
}

// FormatMinutes returns "10 minutes", "45 minutes", etc., for the allowed set.
func FormatMinutes(d time.Duration) string {
	m := int(d.Minutes())
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

// FormatDurationLabel is an alias for FormatMinutes for backward compatibility.
func FormatDurationLabel(d time.Duration) string {
	return FormatMinutes(d)
}
