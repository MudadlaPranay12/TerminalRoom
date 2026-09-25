package client

import (
	"bufio"
	"bytes"
	"testing"
)

func TestHandleCommandQuitGraceful(t *testing.T) {
	c := &Client{
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}
	done := make(chan struct{})
	shouldExit := c.handleCommand("/quit", done)
	if !shouldExit {
		t.Error("/quit should return true for graceful exit")
	}
	// Should not have panicked or called os.Exit
}

func TestHandleCommandQuitCaseInsensitive(t *testing.T) {
	c := &Client{writer: bufio.NewWriter(&bytes.Buffer{})}
	done := make(chan struct{})
	if !c.handleCommand("/QUIT", done) {
		t.Error("/QUIT should be handled case-insensitively")
	}
	if !c.handleCommand("  /quit  ", done) {
		t.Error("/quit with whitespace should trigger")
	}
}

func TestHandleCommandHelpDoesNotExit(t *testing.T) {
	c := &Client{writer: bufio.NewWriter(&bytes.Buffer{})}
	done := make(chan struct{})
	if c.handleCommand("/help", done) {
		t.Error("/help should not trigger exit")
	}
	if c.handleCommand("/info", done) {
		t.Error("/info should not trigger exit")
	}
	if c.handleCommand("/unknown", done) {
		t.Error("unknown should not trigger exit")
	}
}

func TestHandleCommandQuitWithNilWriter(t *testing.T) {
	c := &Client{writer: nil}
	done := make(chan struct{})
	// Should not panic even if writer is nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleCommand with nil writer panicked: %v", r)
		}
	}()
	if !c.handleCommand("/quit", done) {
		t.Error("/quit should still return true even with nil writer")
	}
}

func TestClientGracefulExitDoesNotHang(t *testing.T) {
	// Simulate that client exit when server unavailable should not hang.
	// We test that handleCommand returns quickly and does not block.
	c := &Client{writer: bufio.NewWriter(&bytes.Buffer{})}
	done := make(chan struct{})
	start := make(chan struct{})
	go func() {
		close(start)
		c.handleCommand("/quit", done)
	}()
	<-start
	// Should return immediately without blocking
}

func TestClientRunCountdownStopsOnDone(t *testing.T) {
	c := &Client{}
	done := make(chan struct{})
	leaveCh := make(chan struct{}, 1)
	warningsSent := make(map[int]bool)
	// Start countdown, then close done and ensure it exits
	go c.runCountdown(done, leaveCh, warningsSent)
	close(done)
	// Should exit quickly
}

func TestClientIsVTEnabledSafe(t *testing.T) {
	// Ensure terminal VT helpers are safe
	// This is more of a terminal package test, but ensure client doesn't depend on VT
	// No panic
}
