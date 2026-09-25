package launcher

import (
	"bufio"
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/config"
	"github.com/terminalroom/terminalroom/internal/invitation"
	"github.com/terminalroom/terminalroom/internal/protocol"
	"github.com/terminalroom/terminalroom/internal/tlsutil"
)

// helper to set env isolated

func tempConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("ROOM_TTL", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	// Use localhost to avoid private network dependency in CI
	t.Setenv("SERVER_ADDR", "localhost")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	return cfg
}

func TestResolveModeNoArgsIsLauncher(t *testing.T) {
	if got := ResolveMode(nil); got != ModeLauncher {
		t.Errorf("no args => launcher, got %q", got)
	}
	if got := ResolveMode([]string{}); got != ModeLauncher {
		t.Errorf("empty slice => launcher, got %q", got)
	}
}

func TestResolveModeExplicit(t *testing.T) {
	tests := []struct {
		args []string
		want Mode
	}{
		{[]string{"server"}, ModeServer},
		{[]string{"client"}, ModeClient},
		{[]string{"diagnose"}, ModeDiagnose},
		{[]string{"help"}, ModeHelp},
		{[]string{"--help"}, ModeHelp},
		{[]string{"-h"}, ModeHelp},
		{[]string{"unknown"}, ModeUnknown},
	}
	for _, tc := range tests {
		if got := ResolveMode(tc.args); got != tc.want {
			t.Errorf("ResolveMode(%v)=%q want %q", tc.args, got, tc.want)
		}
	}
}

func TestLauncherUsesConfigCertDir(t *testing.T) {
	_ = tempConfig(t)
	// override cert dir to temp dir to test reuse
	certDir := filepath.Join(t.TempDir(), "certs")
	t.Setenv("CERT_DIR", certDir)
	cfg2, _ := config.Load()
	_ = cfg2.EnsureDirs()
	l, err := New(cfg2)
	if err != nil {
		t.Fatalf("launcher New failed: %v", err)
	}
	defer l.Shutdown()
	if l.Config().CertDir != certDir {
		t.Errorf("launcher should reuse CertDir %q, got %q", certDir, l.Config().CertDir)
	}
	// Ensure cert files exist where expected (server creation generates)
	if _, err := tlsutil.ClientConfig(certDir); err != nil {
		t.Errorf("client config should work with same cert dir: %v", err)
	}
}

func TestLauncherEndpointNotBlindLocalhost(t *testing.T) {
	// When SERVER_ADDR=localhost, endpoint should be 127.0.0.1:port, not blindly localhost:9090?
	// Verify endpoint reflects config port
	_ = tempConfig(t)
	t.Setenv("PORT", "9191")
	cfg2, _ := config.Load()
	_ = cfg2.EnsureDirs()
	l, err := New(cfg2)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Shutdown()
	ep := l.Endpoint()
	if !strings.Contains(ep, "9191") {
		t.Errorf("endpoint should contain configured port 9191, got %q", ep)
	}
	if ep != "127.0.0.1:9191" {
		t.Errorf("endpoint for localhost should be 127.0.0.1:9191, got %q", ep)
	}
}

func TestLauncherRejectsZeroBindingOutsideDevMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "0.0.0.0")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	_, err := New(cfg)
	if err == nil {
		t.Fatal("should reject 0.0.0.0 outside dev mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "0.0.0.0") {
		t.Errorf("error should mention 0.0.0.0, got %q", err.Error())
	}
}

func TestLauncherAllowsZeroInDevMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "0.0.0.0")
	t.Setenv("DEVELOPMENT_MODE", "1")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("0.0.0.0 should be allowed in dev mode, got %v", err)
	}
	defer l.Shutdown()
}

func TestLauncherPrivateAddressResolutionUsed(t *testing.T) {
	// When SERVER_ADDR empty, it should try private discovery; in CI without private net, it should error
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	_, err := New(cfg)
	if err == nil {
		// Some environments have private addresses; if it succeeded, ensure endpoint is private (not 0.0.0.0)
		// But in standard CI it will error
		t.Logf("private network available in this env, launcher created successfully (ok)")
		return
	}
	if !strings.Contains(strings.ToLower(err.Error()), "private network") {
		t.Errorf("should report private network unavailable when no SERVER_ADDR, got %q", err.Error())
	}
}

func TestLauncherPortConflictProducesClearError(t *testing.T) {
	_ = tempConfig(t)
	// Find a free port by creating launcher, then try second on same endpoint
	t.Setenv("PORT", "0") // let OS pick
	// Use explicit localhost with 0 port is not deterministic; instead get free port via net.Listen
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot find free port")
	}
	addr := ln.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	ln.Close()
	t.Setenv("PORT", port)
	cfg2, _ := config.Load()
	_ = cfg2.EnsureDirs()
	l1, err := New(cfg2)
	if err != nil {
		t.Fatalf("first launcher failed: %v", err)
	}
	defer l1.Shutdown()
	l1.Start()
	// Second should fail on same port
	cfg3, _ := config.Load()
	l2, err := New(cfg3)
	if err == nil {
		l2.Shutdown()
		t.Fatal("second launcher should fail due to port conflict")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already in use") && !strings.Contains(strings.ToLower(err.Error()), "port already") {
		t.Errorf("port conflict error should mention 'already in use', got %q", err.Error())
	}
}

func TestLauncherServerStartsBeforeClientDeterministic(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Shutdown()
	start := time.Now()
	l.Start()
	elapsed := time.Since(start)
	// Should not have sleep 1s; readiness is immediate (close channel)
	if elapsed > 500*time.Millisecond {
		t.Errorf("Start took too long (%v), should be deterministic without sleep", elapsed)
	}
	if !l.Server().IsReady() {
		t.Error("server should be ready after Start")
	}
	// Client can connect immediately
	tlsCfg, err := tlsutil.ClientConfig(l.Config().CertDir)
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	conn, err := tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err != nil {
		t.Fatalf("client dial should succeed after server ready: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	_ = protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	msg, err := protocol.Decode(reader)
	if err != nil {
		t.Fatalf("create should get ROOM_ID: %v", err)
	}
	if msg.Type != protocol.MsgRoomID {
		t.Fatalf("expected ROOM_ID, got %s", msg.Type)
	}
}

func TestLauncherWaitReadyChannelWithoutSleep(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Shutdown()
	// Before Start, New already has listener so WaitReady should be ready immediately
	// But per spec, client must not start before readiness; verify channel is closed
	select {
	case <-l.Server().WaitReady():
	default:
		t.Error("WaitReady should be ready immediately after New (deterministic, no sleep)")
	}
	l.Start()
	select {
	case <-l.Server().WaitReady():
	default:
		t.Error("WaitReady should remain ready after Start")
	}
}

func TestLauncherClientExitTriggersServerShutdown(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	l.Start()
	// Simulate client session that creates room then leaves
	tlsCfg, _ := tlsutil.ClientConfig(l.Config().CertDir)
	conn, err := tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	_ = protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	// consume initial messages
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			break
		}
		if msg.Type == protocol.MsgParticipantID {
			break
		}
	}
	// close client
	conn.Close()
	// Now launcher shutdown (simulating client exit)
	l.Shutdown()
	// Second shutdown should be idempotent
	l.Shutdown()
	// Verify server not listening
	_, err = tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err == nil {
		t.Error("server should be shut down after client exit")
	}
}

func TestLauncherServerShutdownIdempotent(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	l.Start()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("shutdown panic: %v", r)
		}
	}()
	l.Shutdown()
	l.Shutdown()
	l.Shutdown()
}

func TestLauncherNoAutomaticRoomCreation(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Shutdown()
	l.Start()
	// After launcher start, there should be zero rooms until client creates one
	if l.Server().Addr() == nil {
		t.Fatal("server addr nil")
	}
	// Active rooms should be 0
	// internal: check via creating a room count? We use server's rooms via test helper? Instead check that no room exists via failing to get any ID
	// We can verify by ensuring a fresh connection hasn't auto-created room by checking that we still need to send CREATE
	tlsCfg, _ := tlsutil.ClientConfig(l.Config().CertDir)
	conn, err := tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	// Without sending CREATE, try to send CHAT - should get ERROR (not auto room)
	_ = protocol.Send(writer, protocol.Message{Type: protocol.MsgChat, Payload: "hello"})
	msg, err := protocol.Decode(reader)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Type != protocol.MsgError {
		t.Errorf("server should not auto-create room; CHAT without CREATE should error, got %s", msg.Type)
	}
}

func TestLauncherActiveRoomCleanedUpOnShutdown(t *testing.T) {
	cfg := tempConfig(t)
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	l.Start()
	tlsCfg, _ := tlsutil.ClientConfig(l.Config().CertDir)
	conn, err := tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	_ = protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	var roomID string
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if msg.Type == protocol.MsgRoomID {
			roomID = msg.Payload
		}
		if msg.Type == protocol.MsgParticipantID {
			break
		}
		_ = roomID
	}
	if roomID == "" {
		t.Fatal("no roomID")
	}
	// Shutdown launcher (simulating app exit with active room)
	l.Shutdown()
	// Give time for cleanup via handleParticipant defer
	time.Sleep(100 * time.Millisecond)
	// After shutdown, new launcher should be able to start on same port after close? Instead check that second dial fails (server gone)
	_, err = tls.Dial("tcp", l.Endpoint(), tlsCfg)
	if err == nil {
		t.Error("server should be gone after shutdown")
	}
	conn.Close()
}

func TestLauncherCertDirReused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	custom := filepath.Join(t.TempDir(), "custom-certs")
	t.Setenv("CERT_DIR", custom)
	t.Setenv("SERVER_ADDR", "localhost")
	t.Setenv("PORT", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	if cfg.CertDir != custom {
		t.Fatalf("certDir should be custom %q, got %q", custom, cfg.CertDir)
	}
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Shutdown()
	// Ensure server cert and client trust use same dir (no InsecureSkipVerify)
	tlsCfg, err := tlsutil.ClientConfig(custom)
	if err != nil {
		t.Fatalf("client config from custom dir: %v", err)
	}
	if tlsCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false")
	}
	if tlsCfg.RootCAs == nil {
		t.Error("RootCAs should be set for pinned cert")
	}
}

func TestIsTraySupportedReturnsFalseWithoutDependency(t *testing.T) {
	if IsTraySupported() {
		t.Error("IsTraySupported should be false in Step 6 without heavy dependency")
	}
}

// F-01: Launcher public SERVER_ADDR rejection
func TestLauncherRejectsPublicServerAddr(t *testing.T) {
	for _, addr := range []string{"8.8.8.8", "1.1.1.1"} {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "")
		t.Setenv("SERVER_ADDR", addr)
		t.Setenv("ENDPOINT_ADDR", "")
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_ = cfg.EnsureDirs()
		_, err := New(cfg)
		if err == nil {
			t.Fatalf("launcher New with SERVER_ADDR=%q should REJECT public", addr)
		}
		low := strings.ToLower(err.Error())
		if !strings.Contains(low, "public") && !strings.Contains(low, "private") {
			t.Logf("public reject error %q does not mention public/private (warning)", err.Error())
		}
	}
}

func TestLauncherAllowsPrivateServerAddrs(t *testing.T) {
	accept := []string{"192.168.1.10", "10.0.0.5", "172.16.1.5", "100.64.10.20", "127.0.0.1", "localhost"}
	for _, addr := range accept {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "")
		t.Setenv("SERVER_ADDR", addr)
		t.Setenv("ENDPOINT_ADDR", "")
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_ = cfg.EnsureDirs()
		// We test ResolveAddresses directly to avoid needing listener for all private IPs
		bind, _, full, err := ResolveAddresses(cfg)
		if err != nil {
			t.Errorf("ResolveAddresses with SERVER_ADDR=%q should ALLOW, got %v", addr, err)
			continue
		}
		if bind == "" || full == "" {
			t.Errorf("ResolveAddresses %q returned empty bind/full", addr)
		}
		// Also test full launcher New for loopback/localhost (does actual listen)
		if addr == "127.0.0.1" || addr == "localhost" {
			l, err := New(cfg)
			if err != nil {
				t.Errorf("launcher New localhost should pass: %v", err)
			} else {
				l.Shutdown()
			}
		}
	}
}

func TestLauncherRejectsPublicHostPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "8.8.8.8:9090")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	_, err := New(cfg)
	if err == nil {
		t.Fatal("SERVER_ADDR=8.8.8.8:9090 should be rejected")
	}
}

func TestLauncherEmptyUsesAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	// Empty should attempt auto-discovery; in CI without private net it errors with private network, which is expected
	_, _, _, err := ResolveAddresses(cfg)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "private network") {
			t.Errorf("empty SERVER_ADDR should either succeed with private or error private network, got %q", err.Error())
		}
	}
}

func TestLauncherPublicRejectedEvenWithHostPortInResolveAddresses(t *testing.T) {
	// Direct ResolveAddresses call with public host:port
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "9090")
	t.Setenv("SERVER_ADDR", "1.1.1.1:9090")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_, _, _, err := ResolveAddresses(cfg)
	if err == nil {
		t.Fatal("ResolveAddresses public host:port should reject")
	}
}

// F-02: ENDPOINT_ADDR validation via launcher path (advertisement/bundling protection)
func TestLauncherAcceptsValidEndpoints(t *testing.T) {
	valid := []string{
		"127.0.0.1:9090",
		"localhost:9090",
		"192.168.1.10:9090",
		"10.0.0.5:9090",
		"172.16.0.10:9090",
		"100.93.120.19:9090",
		"100.64.10.20:9090",
	}
	for _, ep := range valid {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", "localhost")
		t.Setenv("ENDPOINT_ADDR", ep)
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_ = cfg.EnsureDirs()
		_, endpoint, _, err := ResolveAddresses(cfg)
		if err != nil {
			t.Errorf("ENDPOINT_ADDR %q should be accepted, got %v", ep, err)
			continue
		}
		if endpoint != ep {
			t.Errorf("endpoint mismatch: got %q want %q", endpoint, ep)
		}
	}
	// Verify full launcher New advertises correctly for one valid endpoint (use fresh port)
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_, freePort, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	ep := "192.168.1.10:" + freePort
	t.Setenv("PORT", freePort)
	t.Setenv("SERVER_ADDR", "localhost")
	t.Setenv("ENDPOINT_ADDR", ep)
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New with valid ENDPOINT_ADDR %q should succeed: %v", ep, err)
	}
	if l.Endpoint() != ep {
		t.Errorf("launcher Endpoint() = %q want %q", l.Endpoint(), ep)
	}
	l.Shutdown()
}

func TestLauncherRejectsInvalidEndpoints(t *testing.T) {
	invalid := []string{
		"8.8.8.8:9090",
		"1.1.1.1:9090",
		"example.com:9090",
		"192.168.1.10",          // missing port
		"192.168.1.10:0",        // port 0
		"192.168.1.10:65536",    // port >65535
		"0.0.0.0:9090",          // unspecified
		"[::]:9090",             // unspecified IPv6
		"192.168.1.10:abc",      // invalid port
		"192.168.1.10:",         // empty port
		":9090",                 // empty host
		"192.168.1.10:9090:9090", // malformed
	}
	for _, ep := range invalid {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", "localhost")
		t.Setenv("ENDPOINT_ADDR", ep)
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_ = cfg.EnsureDirs()
		_, _, _, err := ResolveAddresses(cfg)
		if err == nil {
			t.Errorf("ENDPOINT_ADDR %q should be REJECTED, got no error", ep)
		}
		// Also via New (advertisement path)
		_, err2 := New(cfg)
		if err2 == nil {
			t.Errorf("New with ENDPOINT_ADDR %q should fail before advertisement", ep)
		}
		if err2 != nil && !strings.Contains(strings.ToLower(err2.Error()), "endpoint") && !strings.Contains(strings.ToLower(err2.Error()), "invalid") {
			t.Logf("ENDPOINT_ADDR %q error %q does not mention endpoint/invalid (warning)", ep, err2.Error())
		}
	}
}

func TestLauncherDevModeDoesNotAllowPublicEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "9090")
	t.Setenv("SERVER_ADDR", "localhost")
	t.Setenv("ENDPOINT_ADDR", "8.8.8.8:9090")
	t.Setenv("DEVELOPMENT_MODE", "1")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	_, _, _, err := ResolveAddresses(cfg)
	if err == nil {
		t.Fatal("DEVELOPMENT_MODE=1 should NOT allow public ENDPOINT_ADDR")
	}
	_, err = New(cfg)
	if err == nil {
		t.Fatal("New with public ENDPOINT_ADDR should fail even in dev mode")
	}
}

func TestLauncherEndpointBundledProtection(t *testing.T) {
	// Valid endpoint is bundled, invalid falls back to bare token (no invalid @ endpoint)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_, freePort, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	ep := "192.168.1.10:" + freePort
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", freePort)
	t.Setenv("SERVER_ADDR", "localhost")
	t.Setenv("ENDPOINT_ADDR", ep)
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("valid endpoint New failed: %v", err)
	}
	defer l.Shutdown()
	gotEp := l.Endpoint()
	if gotEp != ep {
		t.Errorf("expected endpoint %q, got %q", ep, gotEp)
	}
	// Simulate bundling as client does
	token, _ := invitation.GenerateToken()
	bundled := invitation.BundledInvitation(token, gotEp)
	parsedTok, parsedEp, err := invitation.ParseInvite(bundled)
	if err != nil {
		t.Fatalf("bundled valid should parse: %v", err)
	}
	if parsedTok != token || parsedEp != gotEp {
		t.Errorf("bundled parse mismatch: token %q ep %q", parsedTok, parsedEp)
	}
	// Invalid endpoint should not be bundled (BundledInvitation filters)
	invalidEp := "8.8.8.8:9090"
	bundled2 := invitation.BundledInvitation(token, invalidEp)
	if strings.Contains(bundled2, "@") {
		t.Errorf("invalid endpoint should not be bundled, got %q", bundled2)
	}
	if bundled2 != token {
		t.Errorf("invalid endpoint should return bare token, got %q", bundled2)
	}
	// Bare token still valid
	if _, _, err := invitation.ParseInvite(token); err != nil {
		t.Errorf("bare token should remain valid: %v", err)
	}
}

// F-08: 0.0.0.0 host:port handling, whitespace, host:port:port, IPv6 unspecified
func TestF08ServerAddrUnspecifiedHostPort(t *testing.T) {
	// 0.0.0.0:9090 rejected normal, allowed only dev
	for _, tc := range []struct {
		addr string
		dev  string
		shouldPass bool
	}{
		{"0.0.0.0", "", false},
		{"0.0.0.0:9090", "", false},
		{" 0.0.0.0:9090 ", "", false},
		{"0.0.0.0", "1", true},
		{"0.0.0.0:9090", "1", true},
		{" 0.0.0.0:9090 ", "1", true},
	} {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", tc.addr)
		t.Setenv("ENDPOINT_ADDR", "")
		t.Setenv("DEVELOPMENT_MODE", tc.dev)
		cfg, _ := config.Load()
		_ = cfg.EnsureDirs()
		_, _, _, err := ResolveAddresses(cfg)
		if tc.shouldPass && err != nil {
			t.Errorf("SERVER_ADDR %q dev=%q should pass, got %v", tc.addr, tc.dev, err)
		}
		if !tc.shouldPass && err == nil {
			t.Errorf("SERVER_ADDR %q dev=%q should be REJECTED", tc.addr, tc.dev)
		}
		if tc.shouldPass && tc.addr != "" {
			// Also verify dev does not advertise 0.0.0.0
			l, err := New(cfg)
			if err != nil {
				t.Fatalf("dev New %q should succeed: %v", tc.addr, err)
			}
			if l.Endpoint() == "0.0.0.0:9090" || l.Endpoint() == "[::]:9090" {
				t.Errorf("dev endpoint should not be unspecified, got %q", l.Endpoint())
			}
			l.Shutdown()
		}
	}
}

func TestF08HostPortDoubleNotCreated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "9090")
	t.Setenv("SERVER_ADDR", "192.168.1.10:9090")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg, _ := config.Load()
	_ = cfg.EnsureDirs()
	bind, endpoint, full, err := ResolveAddresses(cfg)
	if err != nil {
		t.Fatalf("host:port private should be accepted: %v", err)
	}
	if full != "192.168.1.10:9090" {
		t.Errorf("host:port double check: fullBindAddr = %q want %q", full, "192.168.1.10:9090")
	}
	if endpoint != "192.168.1.10:9090" {
		t.Errorf("endpoint for host:port bind should be same, got %q", endpoint)
	}
	// Whitespace variant
	t.Setenv("SERVER_ADDR", " 192.168.1.10:9090 ")
	cfg2, _ := config.Load()
	bind2, _, full2, err := ResolveAddresses(cfg2)
	if err != nil {
		t.Fatalf("whitespace host:port should be accepted: %v", err)
	}
	if full2 != "192.168.1.10:9090" {
		t.Errorf("whitespace host:port full = %q", full2)
	}
	_ = bind
	_ = bind2
}

func TestF08EndpointWhitespaceAndIPv6Unspecified(t *testing.T) {
	// Whitespace endpoint
	for _, ep := range []string{" 192.168.1.10:9090 ", " 127.0.0.1:9090 "} {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", "localhost")
		t.Setenv("ENDPOINT_ADDR", ep)
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_, endpoint, _, err := ResolveAddresses(cfg)
		if err != nil {
			t.Errorf("whitespace endpoint %q should be accepted, got %v", ep, err)
		} else if strings.TrimSpace(endpoint) != strings.TrimSpace(ep) {
			// endpoint is trimmed via config TrimSpace, should equal trimmed
			t.Errorf("whitespace endpoint mismatch: got %q", endpoint)
		}
	}
	// IPv6 unspecified must be rejected via endpoint
	for _, ep := range []string{"[::]:9090", "0.0.0.0:9090", "8.8.8.8:9090"} {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", "localhost")
		t.Setenv("ENDPOINT_ADDR", ep)
		t.Setenv("DEVELOPMENT_MODE", "")
		cfg, _ := config.Load()
		_, _, _, err := ResolveAddresses(cfg)
		if err == nil {
			t.Errorf("endpoint %q should be REJECTED", ep)
		}
		// Also bundled must not contain it
		tok, _ := invitation.GenerateToken()
		bundled := invitation.BundledInvitation(tok, ep)
		if strings.Contains(bundled, "@") {
			t.Errorf("bundled with unspecified %q should not contain @, got %q", ep, bundled)
		}
	}
	// Valid Tailscale and RFC1918 with whitespace
	for _, ep := range []string{" 10.0.0.5:9090 ", " 100.93.120.19:9090 "} {
		dir := t.TempDir()
		t.Setenv("APPDATA", dir)
		t.Setenv("CERT_DIR", "")
		t.Setenv("PORT", "9090")
		t.Setenv("SERVER_ADDR", "localhost")
		t.Setenv("ENDPOINT_ADDR", ep)
		cfg, _ := config.Load()
		_, endpoint, _, err := ResolveAddresses(cfg)
		if err != nil {
			t.Errorf("valid whitespace endpoint %q should pass: %v", ep, err)
		} else if endpoint != strings.TrimSpace(ep) {
			t.Errorf("endpoint trimming failed: got %q", endpoint)
		}
	}
}
