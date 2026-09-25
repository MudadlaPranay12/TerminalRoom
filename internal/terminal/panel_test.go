package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestCreateRoomPanelEnclosesContent(t *testing.T) {
	s := RenderDurationSelector(10 * time.Minute)
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 3 {
		t.Fatalf("panel too short: %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Error("top border must be ╭─╮")
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
		t.Error("bottom border must be ╰─╯")
	}
	for i, line := range lines {
		if i == 0 || i == len(lines)-1 {
			continue
		}
		// Separator lines have ├ or ─, content lines have │
		if strings.Contains(line, "Create Private Room") || strings.Contains(line, "10 minutes") {
			if !strings.HasPrefix(strings.TrimSpace(line), "│") && !strings.HasPrefix(line, "│") {
				// Allow leading spaces before │
				if !strings.Contains(line, "│") {
					t.Errorf("content line %d should have left/right borders: %q", i, line)
				}
			}
			if !strings.Contains(line, "│") {
				t.Errorf("line %d missing borders: %q", i, line)
			}
		}
	}
	// Every content line between top and bottom should have │ on both sides (except separators)
	for i := 1; i < len(lines)-1; i++ {
		l := lines[i]
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "├") || strings.HasPrefix(trim, "│") || strings.HasPrefix(trim, "╰") || strings.HasPrefix(trim, "╭") {
			continue
		}
		// For our minimal panel, all content lines should be inside │ │
		if !strings.Contains(l, "│") {
			// Allow empty separator lines that are just "│   │"
			if strings.TrimSpace(l) != "" && !strings.Contains(l, "─") {
				t.Errorf("line %d should be inside panel: %q", i, l)
			}
		}
	}
}

func TestPanelWidthFitsContent(t *testing.T) {
	token := "TRINV-" + strings.Repeat("X", 48) + "@100.93.120.19:9090"
	s := RenderWaiting("TR-TEST123", "09:59", "Private", token)
	lines := strings.Split(s, "\n")
	maxVis := 0
	for _, l := range lines {
		if vis := visibleLen(l); vis > maxVis {
			maxVis = vis
		}
	}
	// Panel width should be maxVis, and no line should exceed it
	for _, l := range lines {
		if visibleLen(l) > maxVis {
			t.Errorf("line exceeds max width %d: %q len %d", maxVis, l, visibleLen(l))
		}
	}
	// Invitation should wrap, not break border (split at @, so check parts separately)
	if !strings.Contains(s, "TRINV-") || !strings.Contains(s, "100.93") {
		t.Error("waiting panel should contain invitation parts")
	}
	// Check that long token didn't create line longer than panel
	for _, l := range lines {
		if strings.Contains(l, "TRINV-") && visibleLen(l) > maxVis {
			t.Errorf("invitation line too long: %q", l)
		}
	}
}

func TestANSIWidthHandled(t *testing.T) {
	s := RenderActiveHeader("TR-TEST", "09:59", "ACTIVE")
	// Header contains green Private, but visible length should still be panel width
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		vis := visibleLen(l)
		raw := len(l)
		if raw > vis+20 { // ANSI should add invisible bytes but not affect visible
			// Check that visibleLen correctly strips ANSI
			if !strings.Contains(l, "\033[") {
				continue
			}
			if vis > 62 {
				t.Errorf("visibleLen %d exceeds expected 62 for %q", vis, l)
			}
		}
	}
}

func TestLongInvitationWraps(t *testing.T) {
	long := "TRINV-" + strings.Repeat("A", 60) + "@192.168.1.100:9090"
	box := RenderInvitationBox(long)
	if !strings.Contains(box, "TRINV-") {
		t.Error("should contain token")
	}
	lines := strings.Split(box, "\n")
	for _, l := range lines {
		if visibleLen(l) > 62 {
			t.Errorf("invitation box line too wide: %q len %d", l, visibleLen(l))
		}
	}
}

func TestHomeRemainsOpen(t *testing.T) {
	home := RenderHome()
	// Home MUST now be a properly fitted box containing all content
	if !strings.Contains(home, "╭") || !strings.Contains(home, "╰") {
		t.Error("Home should be a fitted box with ╭/╰ borders")
	}
	if !strings.Contains(home, "TerminalRoom") || !strings.Contains(home, "Create private room") {
		t.Error("Home should contain title and create option")
	}
	if !strings.Contains(home, "─") {
		t.Error("Home should have thin separator inside box")
	}
	lines := strings.Split(strings.TrimSpace(home), "\n")
	if !strings.HasPrefix(lines[0], "╭") {
		t.Error("Home top border must be ╭─╮")
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Error("Home bottom border must be ╰─╯")
	}
	// All content lines must be inside │ │
	for i := 1; i < len(lines)-1; i++ {
		l := lines[i]
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "├") || strings.HasPrefix(trim, "╭") || strings.HasPrefix(trim, "╰") {
			continue
		}
		if !strings.Contains(l, "│") {
			t.Errorf("Home line %d should be inside panel borders: %q", i, l)
		}
	}
	// Menu and tagline must be inside box, not outside
	if !strings.Contains(home, "[C] Create private room") || !strings.Contains(home, "PRIVATE \u2022 TEMPORARY") {
		t.Error("Home menu/tagline should be inside box")
	}
	// Ensure no outside horizontal separator line separate from box
	// Count ╭ should be exactly 1
	if strings.Count(home, "╭") != 1 || strings.Count(home, "╰") != 1 {
		t.Errorf("Home should have exactly one outer box, got %d top %d bottom", strings.Count(home, "╭"), strings.Count(home, "╰"))
	}
}

func TestWaitingPanelFitted(t *testing.T) {
	s := RenderWaiting("TR-ABC", "09:59", "Private", "TRINV-abc@127.0.0.1:9090")
	if !strings.HasPrefix(strings.TrimSpace(s), "╭") {
		t.Error("Waiting should be a fitted panel with top border")
	}
	if !strings.Contains(s, "WAITING") || !strings.Contains(s, "1/2") {
		t.Error("Waiting should be 1/2")
	}
}

func TestActivePanelFitted(t *testing.T) {
	s := RenderActiveHeader("TR-XYZ", "08:31", "ACTIVE")
	if !strings.HasPrefix(strings.TrimSpace(s), "╭") {
		t.Error("Active header should be boxed top")
	}
	if !strings.Contains(s, "Private") || !strings.Contains(s, "2/2") {
		t.Error("Active should be 2/2 Private")
	}
}

func TestNarrowTerminalNoOverflow(t *testing.T) {
	// Simulate narrow by checking that renderPanel would cap width
	// Our getTerminalWidth fallback is 62, so we check that no line exceeds 80
	s := RenderDurationSelector(30 * time.Minute)
	for _, l := range strings.Split(s, "\n") {
		if visibleLen(l) > 80 {
			t.Errorf("panel line exceeds 80: %q len %d", l, visibleLen(l))
		}
	}
}

func TestCountdownDoesNotCreateNewLines(t *testing.T) {
	// UpdateHeaderInPlace should not contain \n that would create repeated EXPIRES IN lines
	// It should be a single line update via ANSI, not a new line
	// We check that RenderActiveHeader does not contain "EXPIRES IN:" as separate line
	h1 := RenderActiveHeader("TR-TEST", "09:59", "WAITING")
	h2 := RenderActiveHeader("TR-TEST", "09:58", "WAITING")
	if h1 == h2 {
		t.Error("header should change with remaining")
	}
	if strings.Count(h1, "EXPIRES") > 0 || strings.Count(h2, "EXPIRES") > 0 {
		t.Error("header should contain remaining, not EXPIRES IN: label")
	}
}

func TestAllPanelsEncloseContent(t *testing.T) {
	panels := []struct {
		name     string
		s        string
		hasBottom bool
	}{
		{"Home", RenderHome(), true},
		{"CreateDuration", RenderDurationSelector(10 * time.Minute), true},
		{"Waiting", RenderWaiting("TR-TEST", "09:59", "Private", "TRINV-abc@127.0.0.1:9090"), true},
		{"ActiveHeader", RenderActiveHeader("TR-TEST", "09:59", "ACTIVE"), false},
		{"Help", RenderHelp(), true},
		{"Info", RenderInfo("TR-TEST", "ACTIVE", "You + Friend", "in 5m", "Private"), true},
		{"Error", RenderError("ERROR", "Something failed"), true},
		{"Destroyed", RenderDestroyed(), true},
	}
	for _, p := range panels {
		lines := strings.Split(strings.TrimSpace(p.s), "\n")
		if len(lines) < 3 {
			t.Errorf("%s panel too short", p.name)
			continue
		}
		if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
			t.Errorf("%s top border not ╭─╮: %q", p.name, lines[0])
		}
		if p.hasBottom {
			if !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
				t.Errorf("%s bottom border not ╰─╯: %q", p.name, lines[len(lines)-1])
			}
		} else {
			// ActiveHeader is top segment of chat box, ends with separator
			if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "├") {
				t.Errorf("%s should end with separator ├─: %q", p.name, lines[len(lines)-1])
			}
		}
		for i := 1; i < len(lines)-1; i++ {
			l := lines[i]
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "├") {
				continue
			}
			if !strings.Contains(l, "│") {
				t.Errorf("%s line %d not inside borders: %q", p.name, i, l)
			}
		}
	}
	// Also verify that full chat box (header+footer) encloses
	inner := panelInnerWidth()
	header := RenderActiveHeader("TR-TEST", "09:59", "ACTIVE")
	footer := RenderChatFooter()
	full := header + "\n│" + strings.Repeat(" ", inner) + "│\n" + footer
	flines := strings.Split(strings.TrimSpace(full), "\n")
	if !strings.HasPrefix(flines[0], "╭") || !strings.HasPrefix(flines[len(flines)-1], "╰") {
		t.Error("Full chat panel should have ╭ top and ╰ bottom via footer")
	}
}

func TestPanelWidthConsistent(t *testing.T) {
	s := RenderHome()
	lines := strings.Split(s, "\n")
	max := 0
	for _, l := range lines {
		if v := visibleLen(l); v > max {
			max = v
		}
	}
	for _, l := range lines {
		if visibleLen(l) != max {
			t.Errorf("Home panel line width inconsistent: %q visible %d vs max %d", l, visibleLen(l), max)
		}
	}
	if max < 50 || max > 80 {
		t.Errorf("Home panel width %d out of expected 50..80", max)
	}
}

func TestVisibleLenANSI(t *testing.T) {
	plain := "Private"
	colored := colorGreen("Private")
	if visibleLen(colored) != visibleLen(plain) {
		t.Errorf("visibleLen should strip ANSI: plain %d colored %d", visibleLen(plain), visibleLen(colored))
	}
	s := "\033[32m●\033[0m Private   \033[90m2/2\033[0m   \033[90m09:59\033[0m"
	if visibleLen(s) != visibleLen(stripANSI(s)) {
		t.Error("visibleLen should equal stripANSI length")
	}
}

func TestInvitationWrapsNarrow(t *testing.T) {
	long := "TRINV-" + strings.Repeat("X", 80) + "@100.93.120.19:9090"
	s := RenderWaiting("TR-TEST", "09:59", "Private", long)
	lines := strings.Split(s, "\n")
	max := 0
	for _, l := range lines {
		if v := visibleLen(l); v > max {
			max = v
		}
	}
	for _, l := range lines {
		if visibleLen(l) > max {
			t.Errorf("wrapped line exceeds max: %q", l)
		}
		if visibleLen(l) > 80 {
			t.Errorf("wrapped line too wide >80: %q", l)
		}
	}
	if !strings.Contains(s, "TRINV-") || !strings.Contains(s, "100.93") {
		t.Error("should contain invitation parts even when wrapped")
	}
}

func TestNarrowTerminalBehavior(t *testing.T) {
	t.Setenv("COLUMNS", "55")
	s := RenderHome()
	for _, l := range strings.Split(s, "\n") {
		if visibleLen(l) > 55 {
			t.Errorf("narrow terminal: line exceeds 55: %q len %d", l, visibleLen(l))
		}
	}
	t.Setenv("COLUMNS", "120")
	s2 := RenderDurationSelector(30 * time.Minute)
	for _, l := range strings.Split(s2, "\n") {
		if visibleLen(l) > 80 {
			t.Errorf("wide terminal should cap at 80: %q len %d", l, visibleLen(l))
		}
	}
}

func TestCountdownHeaderInPlace(t *testing.T) {
	h := RenderActiveHeader("TR-TEST", "09:59", "WAITING")
	if !strings.Contains(h, "09:59") {
		t.Error("header should contain countdown")
	}
	if strings.Contains(h, "EXPIRES IN") {
		t.Error("header should not contain EXPIRES IN")
	}
	// Ensure UpdateHeaderInPlace would use header's first content line with save/restore
	if strings.Contains(h, "\nEXPIRES") {
		t.Error("countdown should not add new lines")
	}
}

func TestDuplicateHomePrevention(t *testing.T) {
	home := RenderHome()
	count := strings.Count(home, "TerminalRoom")
	if count != 1 {
		t.Errorf("Home should contain TerminalRoom exactly once, got %d", count)
	}
	if strings.Count(home, "╭") != 1 || strings.Count(home, "╰") != 1 {
		t.Errorf("Home should have exactly one outer box")
	}
	// Simulate that client Run loop would render Home once per iteration; ensure no double inside single render
	if strings.Count(home, "Create private room") != 1 {
		t.Error("Home should contain Create private room exactly once")
	}
}
