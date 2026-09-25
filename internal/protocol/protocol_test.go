package protocol

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func mustDecode(t *testing.T, s string) *Message {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(s))
	m, err := Decode(r)
	if err != nil {
		t.Fatalf("Decode(%q) failed: %v", s, err)
	}
	return m
}

func TestNormalValidMessage(t *testing.T) {
	m := mustDecode(t, "CHAT|hello\n")
	if m.Type != MsgChat || m.Payload != "hello" {
		t.Errorf("got %v %q", m.Type, m.Payload)
	}
	m2 := mustDecode(t, "CREATE|\n")
	if m2.Type != MsgCreate || m2.Payload != "" {
		t.Errorf("CREATE empty payload failed: %v %q", m2.Type, m2.Payload)
	}
}

func TestPayloadExactlyAtMax(t *testing.T) {
	payload := strings.Repeat("a", MaxPayloadSize)
	msg := Message{Type: MsgChat, Payload: payload}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := Send(w, msg); err != nil {
		t.Fatalf("Send at MaxPayload should succeed: %v", err)
	}
	r := bufio.NewReader(&buf)
	decoded, err := Decode(r)
	if err != nil {
		t.Fatalf("Decode at Max should succeed: %v", err)
	}
	if decoded.Payload != payload {
		t.Error("payload mismatch at max")
	}
	line := msg.Encode()
	if len(line) > MaxLineSize {
		t.Errorf("line at max payload exceeds MaxLineSize: %d > %d", len(line), MaxLineSize)
	}
}

func TestPayloadOneOverMaxRejected(t *testing.T) {
	payload := strings.Repeat("a", MaxPayloadSize+1)
	msg := Message{Type: MsgChat, Payload: payload}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := Send(w, msg); err == nil {
		t.Fatal("Send with payload Max+1 should be rejected")
	} else if !strings.Contains(err.Error(), "payload too large") {
		t.Errorf("expected payload too large, got %v", err)
	}
	// Decode path: construct line with over-max payload directly
	line := "CHAT|" + payload + "\n"
	r := bufio.NewReader(strings.NewReader(line))
	_, err := Decode(r)
	if err == nil {
		t.Fatal("Decode with payload Max+1 should be rejected")
	}
	if !strings.Contains(err.Error(), "payload too large") && !strings.Contains(err.Error(), "message too large") {
		t.Errorf("expected payload/message too large, got %v", err)
	}
}

func TestEmptyPayloadRemainsCompatible(t *testing.T) {
	// CREATE with empty payload is used
	m := mustDecode(t, "CREATE|\n")
	if m.Type != MsgCreate || m.Payload != "" {
		t.Error("empty payload not preserved")
	}
	// JOIN empty token is not valid token but protocol allows empty payload framing
	m2 := mustDecode(t, "JOIN|\n")
	if m2.Payload != "" {
		t.Error("empty JOIN payload")
	}
}

func TestMalformedFrameRejected(t *testing.T) {
	bad := []string{
		"NOPAYLOAD\n",      // no |
		"CHAT hello\n",     // no |
		"|\n",              // empty type? still parts len 2 but type empty - currently allowed? Check: SplitN("|",2) on "|\n" -> ["", ""] len2, would be accepted as Type="" Payload="". Is that malformed? Our code currently would accept empty type as valid Message{Type:"", Payload:""}. That's arguably malformed but not in spec. We test known bad: no pipe
		"CHAT|payload",     // no newline -> incomplete
		"\n",               // empty line
		"CHAT|\r\n",        // payload empty but line is "CHAT|" -> after TrimRight, "CHAT|" -> payload "" valid, not malformed; so skip
	}
	for _, s := range bad {
		r := bufio.NewReader(strings.NewReader(s))
		_, err := Decode(r)
		if err == nil {
			// For "NOPAYLOAD\n" and "CHAT hello\n" should fail malformed
			if s == "NOPAYLOAD\n" || s == "CHAT hello\n" || s == "\n" {
				t.Errorf("malformed %q should be rejected", s)
			}
		}
	}
	// Explicit malformed no pipe
	r := bufio.NewReader(strings.NewReader("NOPAYLOAD\n"))
	if _, err := Decode(r); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("expected malformed, got %v", err)
	}
}

func TestOversizedIncomingRejectedBeforeAllocation(t *testing.T) {
	// Line exceeds MaxLineSize (8192) without needing huge allocation; 9000 is enough to exceed
	oversizedLine := strings.Repeat("A", MaxLineSize+1) + "\n"
	r := bufio.NewReader(strings.NewReader(oversizedLine))
	_, err := Decode(r)
	if err == nil {
		t.Fatal("oversized line should be rejected")
	}
	if !strings.Contains(err.Error(), "message too large") {
		t.Errorf("expected message too large, got %v", err)
	}
	// Payload over max but line within limit: use valid line length but payload 4097
	payload := strings.Repeat("b", MaxPayloadSize+1)
	line2 := "CHAT|" + payload + "\n" // len ~4102+5 <8192 but payload >4096
	r2 := bufio.NewReader(strings.NewReader(line2))
	_, err = Decode(r2)
	if err == nil || !strings.Contains(err.Error(), "payload too large") {
		t.Errorf("expected payload too large for line2, got %v", err)
	}
}

func TestOversizedOutgoingRejected(t *testing.T) {
	msg := Message{Type: MsgChat, Payload: strings.Repeat("x", MaxPayloadSize+1)}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := Send(w, msg); err == nil {
		t.Fatal("oversized Send should fail")
	}
	// Also test line too large via huge Type
	hugeType := MsgType(strings.Repeat("T", MaxLineSize))
	msg2 := Message{Type: hugeType, Payload: "hi"}
	if err := Send(w, msg2); err == nil {
		t.Fatal("oversized line Send should fail")
	}
}

func TestMultipleNormalMessages(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	msgs := []Message{
		{Type: MsgCreate, Payload: ""},
		{Type: MsgJoin, Payload: "TRINV-" + strings.Repeat("a", 48)},
		{Type: MsgChat, Payload: "hello"},
		{Type: MsgChat, Payload: "world"},
	}
	for _, m := range msgs {
		if err := Send(w, m); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}
	r := bufio.NewReader(&buf)
	for _, want := range msgs {
		got, err := Decode(r)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if got.Type != want.Type || got.Payload != want.Payload {
			t.Errorf("mismatch want %v %q got %v %q", want.Type, want.Payload, got.Type, got.Payload)
		}
	}
}

func TestOversizedDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Decode panicked on oversized: %v", r)
		}
	}()
	oversized := strings.Repeat("A", MaxLineSize+100) + "\n"
	r := bufio.NewReader(strings.NewReader(oversized))
	_, _ = Decode(r)
	payload := strings.Repeat("a", MaxPayloadSize+100)
	_, _ = Decode(bufio.NewReader(strings.NewReader("CHAT|" + payload + "\n")))
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = Send(w, Message{Type: MsgChat, Payload: payload})
}

func TestMsgEndpointStillWorks(t *testing.T) {
	ep := "192.168.1.10:9090"
	m := Message{Type: MsgEndpoint, Payload: ep}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := Send(w, m); err != nil {
		t.Fatalf("endpoint Send failed: %v", err)
	}
	r := bufio.NewReader(&buf)
	got, err := Decode(r)
	if err != nil || got.Payload != ep || got.Type != MsgEndpoint {
		t.Fatalf("endpoint decode failed: %v %v", got, err)
	}
}

func TestInvitationAndChatStillWork(t *testing.T) {
	token := "TRINV-" + strings.Repeat("a", 48)
	m := Message{Type: MsgJoin, Payload: token}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := Send(w, m); err != nil {
		t.Fatalf("invitation Send failed: %v", err)
	}
	r := bufio.NewReader(&buf)
	got, err := Decode(r)
	if err != nil || got.Payload != token {
		t.Fatalf("invitation decode failed")
	}
	chat := Message{Type: MsgChat, Payload: "hello friend"}
	buf.Reset()
	w = bufio.NewWriter(&buf)
	if err := Send(w, chat); err != nil {
		t.Fatalf("chat Send failed: %v", err)
	}
	r = bufio.NewReader(&buf)
	got, err = Decode(r)
	if err != nil || got.Payload != "hello friend" {
		t.Fatalf("chat decode failed")
	}
}

func TestEmptyLineAfterTrim(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	if _, err := Decode(r); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty message error, got %v", err)
	}
	r2 := bufio.NewReader(strings.NewReader("\r\n"))
	if _, err := Decode(r2); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty for CRLF empty, got %v", err)
	}
}
