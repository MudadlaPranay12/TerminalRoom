package invitation

import "testing"

func TestValidateEndpointULA(t *testing.T) {
	valid := []string{
		"[fc00::1]:9090",
		"[fd00::1]:9090",
		"[fd12:3456:789a::1]:9090",
		"[fd00::1]:8080",
	}
	for _, ep := range valid {
		if err := ValidateEndpoint(ep); err != nil {
			t.Errorf("ValidateEndpoint ULA %q should pass: %v", ep, err)
		}
	}
	invalid := []string{
		"[::]:9090", // unspecified
		"[2001:4860:4860::8888]:9090", // public
		"[2001:db8::1]:9090",          // public documentation
	}
	for _, ep := range invalid {
		if err := ValidateEndpoint(ep); err == nil {
			t.Errorf("ValidateEndpoint %q should be rejected", ep)
		}
	}
}

func TestParseInviteWithULA(t *testing.T) {
	token, _ := GenerateToken()
	ulaEp := "[fd00::1]:9090"
	bundled := BundledInvitation(token, ulaEp)
	pt, ep, err := ParseInvite(bundled)
	if err != nil {
		t.Fatalf("ParseInvite ULA bundled failed: %v", err)
	}
	if pt != token || ep != ulaEp {
		t.Errorf("ULA bundled parse mismatch")
	}
	// Public IPv6 should be rejected in bundled
	publicBundled := token + "@[2001:4860:4860::8888]:9090"
	if _, _, err := ParseInvite(publicBundled); err == nil {
		t.Error("public IPv6 bundled should be rejected")
	}
	// Unspecified IPv6 bundled should be rejected
	unspecBundled := token + "@[::]:9090"
	if _, _, err := ParseInvite(unspecBundled); err == nil {
		t.Error("unspecified IPv6 bundled should be rejected")
	}
}
