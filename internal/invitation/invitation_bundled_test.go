package invitation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 1. Bare invitation parses successfully.
func TestParseInviteBare(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	parsedToken, endpoint, err := ParseInvite(token)
	if err != nil {
		t.Fatalf("ParseInvite bare failed: %v", err)
	}
	if parsedToken != token {
		t.Errorf("bare token mismatch: got %q want %q", parsedToken, token)
	}
	if endpoint != "" {
		t.Errorf("bare should have no endpoint, got %q", endpoint)
	}
}

// 2. Bundled invitation parses successfully.
func TestParseInviteBundled(t *testing.T) {
	token, _ := GenerateToken()
	bundled := BundledInvitation(token, "100.64.0.10:9090")
	parsedToken, endpoint, err := ParseInvite(bundled)
	if err != nil {
		t.Fatalf("ParseInvite bundled failed: %v", err)
	}
	if parsedToken != token {
		t.Errorf("bundled token mismatch")
	}
	if endpoint != "100.64.0.10:9090" {
		t.Errorf("endpoint = %q want %q", endpoint, "100.64.0.10:9090")
	}
}

// 3. Token portion preserved exactly.
func TestParseInviteTokenPreserved(t *testing.T) {
	token, _ := GenerateToken()
	bundled := token + "@192.168.1.50:9090"
	pt, ep, err := ParseInvite(bundled)
	if err != nil {
		t.Fatal(err)
	}
	if pt != token {
		t.Errorf("token not preserved")
	}
	if ep != "192.168.1.50:9090" {
		t.Errorf("endpoint not preserved")
	}
}

// 4. Endpoint extracted correctly (various private endpoints).
func TestParseInviteEndpointExtraction(t *testing.T) {
	token, _ := GenerateToken()
	cases := []string{
		"100.64.0.10:9090",
		"192.168.1.50:9090",
		"10.0.0.5:8080",
		"172.16.0.1:9090",
		"127.0.0.1:9090",
		"localhost:9090",
	}
	for _, ep := range cases {
		bundled := BundledInvitation(token, ep)
		_, gotEp, err := ParseInvite(bundled)
		if err != nil {
			t.Errorf("ParseInvite %q failed: %v", bundled, err)
			continue
		}
		if gotEp != ep {
			t.Errorf("endpoint extraction %q got %q", ep, gotEp)
		}
	}
}

// 5. Surrounding whitespace trimmed.
func TestParseInviteWhitespaceTrimmed(t *testing.T) {
	token, _ := GenerateToken()
	bundled := "  " + token + "@100.64.0.10:9090" + "  \n"
	pt, ep, err := ParseInvite(bundled)
	if err != nil {
		t.Fatalf("whitespace should be trimmed: %v", err)
	}
	if pt != token || ep != "100.64.0.10:9090" {
		t.Errorf("whitespace trim failed: token %q ep %q", pt, ep)
	}
	// Bare with whitespace
	pt2, ep2, err := ParseInvite("  " + token + "  ")
	if err != nil {
		t.Fatalf("bare whitespace trim failed: %v", err)
	}
	if pt2 != token || ep2 != "" {
		t.Errorf("bare whitespace trim failed")
	}
}

// 6. Malformed invitation rejected.
func TestParseInviteMalformedRejected(t *testing.T) {
	malformed := []string{
		"",
		"   ",
		"TRINV-",
		"TRINV-short",
		"TRINV-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", // non-hex
		"TRINV-abc@extra@100.64.0.10:9090",                       // multiple @
		"TRINV-abc@",
		"@100.64.0.10:9090",
		"TRINV-abc@@100.64.0.10:9090",
		"randomtext",
	}
	for _, s := range malformed {
		if _, _, err := ParseInvite(s); err == nil {
			t.Errorf("malformed %q should be rejected", s)
		}
	}
}

// 7. Malformed endpoint rejected.
func TestParseInviteMalformedEndpointRejected(t *testing.T) {
	token, _ := GenerateToken()
	cases := []string{
		token + "@",                  // empty endpoint
		token + "@:9090",             // empty host
		token + "@100.64.0.10",       // missing port
		token + "@100.64.0.10:",      // empty port
		token + "@: ",                // malformed
		token + "@100.64.0.10:abc",   // non-numeric port
		token + "@100.64.0.10:0",     // zero port
		token + "@100.64.0.10:70000", // port too large
		token + "@[::1:9090",         // malformed bracket
	}
	for _, s := range cases {
		if _, _, err := ParseInvite(s); err == nil {
			t.Errorf("malformed endpoint %q should be rejected", s)
		}
	}
}

// 8. Public/invalid endpoint rejected where network policy requires.
func TestParseInvitePublicRejected(t *testing.T) {
	token, _ := GenerateToken()
	public := []string{
		token + "@8.8.8.8:9090",
		token + "@1.1.1.1:9090",
		token + "@203.0.113.1:9090",
		token + "@100.63.0.1:9090", // 100.63 is not tailscale
		token + "@172.15.0.1:9090", // 172.15 not private
		token + "@192.0.2.1:9090",
		token + "@example.com:9090", // DNS not allowed except localhost
	}
	for _, s := range public {
		if _, _, err := ParseInvite(s); err == nil {
			t.Errorf("public endpoint %q should be rejected", s)
		}
	}
}

// 9. 0.0.0.0 rejected.
func TestParseInviteZeroRejected(t *testing.T) {
	token, _ := GenerateToken()
	cases := []string{
		token + "@0.0.0.0:9090",
		token + "@0.0.0.0:1",
	}
	for _, s := range cases {
		if _, _, err := ParseInvite(s); err == nil {
			t.Errorf("0.0.0.0 endpoint %q should be rejected", s)
		}
	}
	// Also direct ValidateEndpoint
	if err := ValidateEndpoint("0.0.0.0:9090"); err == nil {
		t.Error("ValidateEndpoint 0.0.0.0:9090 should fail")
	}
}

// 10. Existing token validation still works.
func TestTokenValidationStillWorks(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-ROOM1", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Bare token validate
	if _, err := m.ValidateByToken(inv.Token); err != nil {
		t.Errorf("bare token should validate: %v", err)
	}
	// Second use should fail (single-use)
	m2 := NewManager()
	inv2, _ := m2.Create("TR-ROOM2", 10*time.Minute)
	bundled := BundledInvitation(inv2.Token, "192.168.1.50:9090")
	pt, ep, err := ParseInvite(bundled)
	if err != nil {
		t.Fatalf("bundled parse failed: %v", err)
	}
	if ep != "192.168.1.50:9090" {
		t.Errorf("endpoint mismatch")
	}
	// Validate via extracted token
	if _, err := m2.ValidateByToken(pt); err != nil {
		t.Errorf("extracted token should validate: %v", err)
	}
}

// 11. Token remains auth credential (endpoint not used for auth).
func TestEndpointNotAuth(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	bundled := BundledInvitation(inv.Token, "192.168.1.50:9090")
	pt, ep, _ := ParseInvite(bundled)
	if ep != "192.168.1.50:9090" {
		t.Fatal("endpoint parse failed")
	}
	// Tampering endpoint should not affect token auth
	bundled2 := BundledInvitation(inv.Token, "100.64.0.10:9090")
	pt2, ep2, _ := ParseInvite(bundled2)
	if pt != pt2 {
		t.Error("token should be same regardless of endpoint")
	}
	if ep2 != "100.64.0.10:9090" {
		t.Error("endpoint should differ")
	}
	// Validate token alone (first use consumes, second should fail regardless of endpoint)
	m2 := NewManager()
	inv2, _ := m2.Create("TR-ROOM2", 10*time.Minute)
	pt3, _, _ := ParseInvite(BundledInvitation(inv2.Token, "192.168.1.50:9090"))
	if _, err := m2.ValidateByToken(pt3); err != nil {
		t.Fatalf("first validate should succeed: %v", err)
	}
	// Second attempt with different endpoint but same token should fail (already used)
	pt4, _, _ := ParseInvite(BundledInvitation(inv2.Token, "100.64.0.10:9090"))
	if _, err := m2.ValidateByToken(pt4); err == nil {
		t.Error("second use should fail even with different endpoint")
	}
}

// 12. Endpoint cannot change room authorization.
func TestEndpointCannotChangeRoomBinding(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-ROOM1", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	bundled := BundledInvitation(inv.Token, "192.168.1.50:9090")
	pt, _, _ := ParseInvite(bundled)
	// Correct room
	if _, err := m.Validate(pt, "TR-ROOM1"); err != nil {
		t.Errorf("correct room should succeed: %v", err)
	}
	// Recreate for second test (since previous consumed)
	m2 := NewManager()
	inv2, _ := m2.Create("TR-ROOM1", 10*time.Minute)
	pt2, _, _ := ParseInvite(BundledInvitation(inv2.Token, "192.168.1.50:9090"))
	if _, err := m2.Validate(pt2, "TR-OTHER"); err == nil {
		t.Error("wrong room should fail even with valid endpoint")
	}
}

// 13. Bare-token JOIN remains compatible (server side)
func TestBareTokenJoinCompatible(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	// Simulate old client sending bare token
	if _, err := m.ValidateByToken(inv.Token); err != nil {
		t.Errorf("bare token should still validate: %v", err)
	}
}

// 15. One-time consumption remains atomic (via invitation manager)
func TestOneTimeConsumptionAtomicWithBundled(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	bundled := BundledInvitation(inv.Token, "192.168.1.50:9090")
	pt, _, _ := ParseInvite(bundled)
	// First succeeds
	if _, err := m.ValidateByToken(pt); err != nil {
		t.Fatalf("first should succeed: %v", err)
	}
	// Second with same token (even if re-bundled) fails
	pt2, _, _ := ParseInvite(bundled)
	if _, err := m.ValidateByToken(pt2); err == nil {
		t.Error("second should fail (already used)")
	}
}

// 16. Expiration remains intact (bundled doesn't bypass)
func TestExpirationWithBundled(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", -1*time.Minute) // already expired
	bundled := BundledInvitation(inv.Token, "192.168.1.50:9090")
	pt, _, _ := ParseInvite(bundled)
	if _, err := m.ValidateByToken(pt); err == nil {
		t.Error("expired should fail even when bundled")
	}
}

// 17. Room binding remains intact (bundled)
func TestRoomBindingWithBundled(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	bundled := BundledInvitation(inv.Token, "192.168.1.50:9090")
	pt, _, _ := ParseInvite(bundled)
	if _, err := m.Validate(pt, "TR-ROOM2"); err == nil {
		t.Error("room mismatch should fail")
	}
}

// 18. No disk write (invitation manager is memory only)
func TestNoDiskWrite(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	_ = BundledInvitation(inv.Token, "100.64.0.10:9090")
	// Ensure no files created in temp dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("invitation should not write to disk, found %d files", len(entries))
	}
	// Also check that config dir not polluted
	configPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		t.Error("invitation should not create config.json")
	}
}

// Additional: BundledInvitation helper
func TestBundledInvitationHelper(t *testing.T) {
	token, _ := GenerateToken()
	if got := BundledInvitation(token, ""); got != token {
		t.Errorf("empty endpoint should return token alone")
	}
	if got := BundledInvitation(token, "192.168.1.50:9090"); got != token+"@192.168.1.50:9090" {
		t.Errorf("bundled helper failed")
	}
}

// Additional: IsValidTokenFormat
func TestIsValidTokenFormat(t *testing.T) {
	token, _ := GenerateToken()
	if !IsValidTokenFormat(token) {
		t.Error("generated token should be valid")
	}
	if IsValidTokenFormat("TRINV-short") {
		t.Error("short token should be invalid")
	}
	if IsValidTokenFormat("TRINV-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz") {
		t.Error("non-hex should be invalid")
	}
}

// 8 extended: ValidateEndpoint private vs public
func TestValidateEndpointPrivateVsPublic(t *testing.T) {
	valid := []string{
		"100.64.0.10:9090",
		"100.127.255.255:9090",
		"10.0.0.1:9090",
		"192.168.1.50:9090",
		"172.16.0.1:9090",
		"127.0.0.1:9090",
		"localhost:9090",
		"[::1]:9090",
	}
	for _, ep := range valid {
		if err := ValidateEndpoint(ep); err != nil {
			t.Errorf("valid endpoint %q should pass: %v", ep, err)
		}
	}
	invalid := []string{
		"8.8.8.8:9090",
		"1.1.1.1:9090",
		"0.0.0.0:9090",
		"example.com:9090",
		"192.168.1.50", // missing port
		":9090",
	}
	for _, ep := range invalid {
		if err := ValidateEndpoint(ep); err == nil {
			t.Errorf("invalid endpoint %q should fail", ep)
		}
	}
}

// 14. Bundled-token JOIN connects to embedded endpoint (simulated)
func TestBundledTokenJoinUsesEndpoint(t *testing.T) {
	m := NewManager()
	inv, _ := m.Create("TR-ROOM1", 10*time.Minute)
	endpoint := "100.64.0.10:9090"
	bundled := BundledInvitation(inv.Token, endpoint)
	token, ep, err := ParseInvite(bundled)
	if err != nil {
		t.Fatalf("ParseInvite failed: %v", err)
	}
	if ep != endpoint {
		t.Fatalf("endpoint mismatch: got %q want %q", ep, endpoint)
	}
	// Endpoint should be valid for dialing
	if err := ValidateEndpoint(ep); err != nil {
		t.Fatalf("endpoint should be valid: %v", err)
	}
	// Token should still authenticate
	if _, err := m.ValidateByToken(token); err != nil {
		t.Fatalf("token should validate: %v", err)
	}
	// Simulate second client trying same bundled token should fail (single-use)
	if _, _, err := ParseInvite(bundled); err != nil {
		t.Fatalf("second parse should still succeed")
	}
	if _, err := m.ValidateByToken(token); err == nil {
		t.Error("second use should fail (already consumed)")
	}
}

// F-02: BundledInvitation must not embed invalid endpoint; bare token remains valid
func TestBundledInvitationFiltersInvalidEndpoint(t *testing.T) {
	token, _ := GenerateToken()
	invalid := []string{
		"8.8.8.8:9090",
		"1.1.1.1:9090",
		"example.com:9090",
		"0.0.0.0:9090",
		"[::]:9090",
		"192.168.1.10",       // missing port
		"192.168.1.10:0",     // port 0
		"192.168.1.10:65536", // port overflow
	}
	for _, ep := range invalid {
		bundled := BundledInvitation(token, ep)
		if strings.Contains(bundled, "@") {
			t.Errorf("BundledInvitation with invalid %q should return bare token, got %q", ep, bundled)
		}
		if bundled != token {
			t.Errorf("invalid endpoint %q should yield bare token %q, got %q", ep, token, bundled)
		}
		// Bare token still parses and authenticates
		if pt, e, err := ParseInvite(bundled); err != nil || pt != token || e != "" {
			t.Errorf("bare token parse after invalid endpoint filter failed: %v", err)
		}
	}
	// Valid endpoint must be bundled
	valid := []string{"127.0.0.1:9090", "localhost:9090", "192.168.1.10:9090", "10.0.0.5:9090", "172.16.0.10:9090", "100.93.120.19:9090"}
	for _, ep := range valid {
		bundled := BundledInvitation(token, ep)
		if bundled != token+"@"+ep {
			t.Errorf("valid endpoint %q should be bundled, got %q", ep, bundled)
		}
		pt, gotEp, err := ParseInvite(bundled)
		if err != nil || pt != token || gotEp != ep {
			t.Errorf("valid bundled parse failed for %q: %v", ep, err)
		}
	}
}
