package network

import (
	"net"
	"testing"
)

func TestClassifyAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected NetworkType
	}{
		{"Tailscale", net.IPv4(100, 64, 0, 1), NetworkTailscale},
		{"Tailscale mid", net.IPv4(100, 96, 0, 1), NetworkTailscale},
		{"Tailscale high", net.IPv4(100, 127, 255, 255), NetworkTailscale},
		{"RFC1918 10.x", net.IPv4(10, 0, 0, 1), NetworkPrivate},
		{"RFC1918 172.16.x", net.IPv4(172, 16, 0, 1), NetworkPrivate},
		{"RFC1918 172.31.x", net.IPv4(172, 31, 255, 255), NetworkPrivate},
		{"RFC1918 192.168.x", net.IPv4(192, 168, 1, 1), NetworkPrivate},
		{"Loopback", net.IPv4(127, 0, 0, 1), NetworkLoopback},
		{"Public", net.IPv4(8, 8, 8, 8), NetworkNone},
		{"Not tailscale", net.IPv4(100, 63, 0, 1), NetworkNone},
		{"Not tailscale high", net.IPv4(100, 128, 0, 1), NetworkNone},
		{"172.15.x not private", net.IPv4(172, 15, 0, 1), NetworkNone},
		{"172.32.x not private", net.IPv4(172, 32, 0, 1), NetworkNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyAddress(tt.ip)
			if result != tt.expected {
				t.Errorf("classifyAddress(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsTailscaleIP(t *testing.T) {
	tests := []struct {
		ip       net.IP
		expected bool
	}{
		{net.IPv4(100, 64, 0, 1), true},
		{net.IPv4(100, 96, 0, 1), true},
		{net.IPv4(100, 127, 255, 255), true},
		{net.IPv4(100, 63, 0, 1), false},
		{net.IPv4(100, 128, 0, 1), false},
		{net.IPv4(10, 0, 0, 1), false},
		{net.IPv4(8, 8, 8, 8), false},
	}

	for _, tt := range tests {
		result := isTailscaleIP(tt.ip)
		if result != tt.expected {
			t.Errorf("isTailscaleIP(%s) = %v, want %v", tt.ip, result, tt.expected)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       net.IP
		expected bool
	}{
		{net.IPv4(10, 0, 0, 1), true},
		{net.IPv4(10, 255, 255, 255), true},
		{net.IPv4(172, 16, 0, 1), true},
		{net.IPv4(172, 31, 255, 255), true},
		{net.IPv4(192, 168, 0, 1), true},
		{net.IPv4(192, 168, 255, 255), true},
		{net.IPv4(172, 15, 0, 1), false},
		{net.IPv4(172, 32, 0, 1), false},
		{net.IPv4(8, 8, 8, 8), false},
		{net.IPv4(127, 0, 0, 1), false},
	}

	for _, tt := range tests {
		result := isPrivateIP(tt.ip)
		if result != tt.expected {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, result, tt.expected)
		}
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		endpoint   string
		wantHost   string
		wantPort   string
		wantErr    bool
	}{
		{"100.64.0.1:9090", "100.64.0.1", "9090", false},
		{"localhost:8080", "localhost", "8080", false},
		{"192.168.1.100:9090", "192.168.1.100", "9090", false},
		{"10.0.0.1", "10.0.0.1", "9090", false},
		{"", "", "9090", false},
	}

	for _, tt := range tests {
		host, port, err := ParseEndpoint(tt.endpoint)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			continue
		}
		if host != tt.wantHost {
			t.Errorf("ParseEndpoint(%q) host = %q, want %q", tt.endpoint, host, tt.wantHost)
		}
		if port != tt.wantPort {
			t.Errorf("ParseEndpoint(%q) port = %q, want %q", tt.endpoint, port, tt.wantPort)
		}
	}
}

func TestResolveBindAddressExplicit(t *testing.T) {
	tests := []struct {
		addr string
		port string
		want string
	}{
		{"localhost", "8080", "127.0.0.1:8080"},
		{"127.0.0.1", "9090", "127.0.0.1:9090"},
		{"192.168.1.100", "9090", "192.168.1.100:9090"},
	}

	for _, tt := range tests {
		result, err := ResolveBindAddress(tt.addr, tt.port)
		if err != nil {
			t.Errorf("ResolveBindAddress(%q, %q) error = %v", tt.addr, tt.port, err)
			continue
		}
		if result != tt.want {
			t.Errorf("ResolveBindAddress(%q, %q) = %q, want %q", tt.addr, tt.port, result, tt.want)
		}
	}
}

func TestResolveBindAddressEmptyReturnsError(t *testing.T) {
	_, err := DiscoverPrivateAddress()
	if err == nil {
		t.Skip("Private network available — cannot test empty-addr error path on this machine")
	}

	_, err = ResolveBindAddress("", "9090")
	if err == nil {
		t.Error("ResolveBindAddress(\"\", \"9090\") should return error when no private network available")
	}
}

func TestResolveBindAddressRejectsZeroBind(t *testing.T) {
	_, err := ResolveBindAddress("0.0.0.0", "9090")
	if err == nil {
		t.Error("ResolveBindAddress(\"0.0.0.0\", \"9090\") should return error")
	}
}

func TestResolveEndpointAddressExplicit(t *testing.T) {
	tests := []struct {
		addr string
		port string
		want string
	}{
		{"100.64.0.1", "9090", "100.64.0.1:9090"},
		{"localhost", "8080", "127.0.0.1:8080"},
		{"127.0.0.1", "9090", "127.0.0.1:9090"},
		{"192.168.1.100", "9090", "192.168.1.100:9090"},
	}

	for _, tt := range tests {
		result, err := ResolveEndpointAddress(tt.addr, tt.port)
		if err != nil {
			t.Errorf("ResolveEndpointAddress(%q, %q) error = %v", tt.addr, tt.port, err)
			continue
		}
		if result != tt.want {
			t.Errorf("ResolveEndpointAddress(%q, %q) = %q, want %q", tt.addr, tt.port, result, tt.want)
		}
	}
}

func TestResolveEndpointAddressZeroBindsToPrivate(t *testing.T) {
	result, err := ResolveEndpointAddress("0.0.0.0", "9090")
	if err != nil {
		t.Skipf("No private network available: %v", err)
	}
	if result == "0.0.0.0:9090" {
		t.Error("Endpoint should not advertise 0.0.0.0 when private address is available")
	}
}

func TestDiscoverPrivateAddress(t *testing.T) {
	pa, err := DiscoverPrivateAddress()
	if err != nil {
		t.Skipf("No private network available (expected in CI): %v", err)
	}

	if pa == nil {
		t.Fatal("DiscoverPrivateAddress returned nil without error")
	}

	if pa.Address == "" {
		t.Error("PrivateAddress.Address is empty")
	}

	if pa.InterfaceName == "" {
		t.Error("PrivateAddress.InterfaceName is empty")
	}

	if pa.NetworkType == NetworkNone {
		t.Error("PrivateAddress.NetworkType should not be NetworkNone")
	}

	ip := net.ParseIP(pa.Address)
	if ip == nil {
		t.Errorf("PrivateAddress.Address is not a valid IP: %s", pa.Address)
	}
}

func TestDiagnosticString(t *testing.T) {
	d := Diagnostic{
		Available:   true,
		Address:     "100.64.0.1:9090",
		Interface:   "Tailscale",
		NetworkType: "tailscale",
	}
	s := d.String()
	if s == "" {
		t.Error("Diagnostic.String() returned empty string")
	}

	d2 := Diagnostic{
		Available: false,
		Error:     "no interfaces found",
	}
	s2 := d2.String()
	if s2 == "" {
		t.Error("Diagnostic.String() returned empty string for error case")
	}

	d3 := Diagnostic{
		Available: false,
	}
	s3 := d3.String()
	if s3 == "" {
		t.Error("Diagnostic.String() returned empty string for unavailable case")
	}
}

func TestNetworkTypeString(t *testing.T) {
	tests := []struct {
		nt       NetworkType
		expected string
	}{
		{NetworkNone, "none"},
		{NetworkTailscale, "tailscale"},
		{NetworkPrivate, "private"},
		{NetworkLoopback, "loopback"},
	}

	for _, tt := range tests {
		if got := tt.nt.String(); got != tt.expected {
			t.Errorf("NetworkType(%d).String() = %q, want %q", tt.nt, got, tt.expected)
		}
	}
}

func TestResolveBindAddressPrivateNetwork(t *testing.T) {
	pa, err := DiscoverPrivateAddress()
	if err != nil {
		t.Skipf("No private network available: %v", err)
	}

	result, err := ResolveBindAddress("", "9090")
	if err != nil {
		t.Fatalf("ResolveBindAddress(\"\", \"9090\") error = %v", err)
	}

	expected := pa.Address + ":9090"
	if result != expected {
		t.Errorf("ResolveBindAddress(\"\", \"9090\") = %q, want %q", result, expected)
	}
}

// F-01: Explicit SERVER_ADDR public rejection
func TestResolveBindAddressAcceptsPrivateAndTailscale(t *testing.T) {
	accept := []string{
		"192.168.1.10",
		"10.0.0.5",
		"172.16.1.5",
		"172.16.5.20",
		"172.31.255.1",
		"192.168.1.5",
		"100.64.10.20",
		"100.64.0.1",
		"100.127.255.1",
	}
	for _, addr := range accept {
		if _, err := ResolveBindAddress(addr, "9090"); err != nil {
			t.Errorf("ResolveBindAddress(%q) should ALLOW private/tailscale, got error %v", addr, err)
		}
	}
}

func TestResolveBindAddressAcceptsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "localhost"} {
		got, err := ResolveBindAddress(addr, "9090")
		if err != nil {
			t.Errorf("ResolveBindAddress(%q) should ALLOW loopback, got %v", addr, err)
		}
		if got != "127.0.0.1:9090" {
			t.Errorf("ResolveBindAddress(%q) = %q want %q", addr, got, "127.0.0.1:9090")
		}
	}
}

func TestResolveBindAddressRejectsPublic(t *testing.T) {
	for _, addr := range []string{"8.8.8.8", "1.1.1.1", "203.0.113.1", "100.63.0.1", "172.15.0.1", "172.32.0.1"} {
		if _, err := ResolveBindAddress(addr, "9090"); err == nil {
			t.Errorf("ResolveBindAddress(%q) should REJECT public, got no error", addr)
		} else if !containsPublic(err.Error()) {
			// Ensure error mentions public/private to aid user
			t.Logf("ResolveBindAddress(%q) error %q does not mention public/private (acceptable but warning)", addr, err.Error())
		}
	}
}

func containsPublic(s string) bool {
	s = lower(s)
	return containsStr(s, "public") || containsStr(s, "private")
}

func lower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsStr(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestValidateBindHost(t *testing.T) {
	// Direct unit for helper including host:port forms
	if err := ValidateBindHost("192.168.1.10"); err != nil {
		t.Errorf("ValidateBindHost private should pass: %v", err)
	}
	if err := ValidateBindHost("192.168.1.10:9090"); err != nil {
		t.Errorf("ValidateBindHost private host:port should pass: %v", err)
	}
	if err := ValidateBindHost("10.0.0.5"); err != nil {
		t.Errorf("ValidateBindHost 10/8 should pass: %v", err)
	}
	if err := ValidateBindHost("100.64.10.20"); err != nil {
		t.Errorf("ValidateBindHost tailscale should pass: %v", err)
	}
	if err := ValidateBindHost("127.0.0.1"); err != nil {
		t.Errorf("ValidateBindHost loopback should pass: %v", err)
	}
	if err := ValidateBindHost("localhost"); err != nil {
		t.Errorf("ValidateBindHost localhost should pass: %v", err)
	}
	if err := ValidateBindHost("8.8.8.8"); err == nil {
		t.Error("ValidateBindHost public should reject")
	}
	if err := ValidateBindHost("8.8.8.8:9090"); err == nil {
		t.Error("ValidateBindHost public host:port should reject")
	}
	if err := ValidateBindHost("0.0.0.0"); err == nil {
		t.Error("ValidateBindHost 0.0.0.0 should reject")
	}
	if err := ValidateBindHost("0.0.0.0:9090"); err == nil {
		t.Error("ValidateBindHost 0.0.0.0:9090 should reject")
	}
	if err := ValidateBindHost(""); err != nil {
		t.Errorf("ValidateBindHost empty should pass (auto): %v", err)
	}
}

// IPv6 ULA fc00::/7
func TestClassifyULA(t *testing.T) {
	tests := []struct {
		ip       string
		expected NetworkType
	}{
		{"fc00::1", NetworkPrivate},
		{"fd12:3456:789a::1", NetworkPrivate},
		{"fd00::1", NetworkPrivate},
		{"fc00::", NetworkPrivate},
		{"::1", NetworkLoopback},
		{"::", NetworkNone}, // unspecified is not private
		{"2001:4860:4860::8888", NetworkNone},
		{"2001:db8::1", NetworkNone},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("ParseIP %q failed", tt.ip)
		}
		got := ClassifyAddress(ip)
		if got != tt.expected {
			t.Errorf("ClassifyAddress(%q) = %v, want %v", tt.ip, got, tt.expected)
		}
		if got == NetworkPrivate && !IsPrivateIP(ip) && !IsULAIP(ip) {
			// ULA should be considered private via IsULAIP or IsPrivateIP
			// For ULA, IsPrivateIP is false (IPv4 only) but IsULAIP true
			if !IsULAIP(ip) {
				t.Errorf("ULA %q should be IsULAIP", tt.ip)
			}
		}
	}
}

func TestIsULAIP(t *testing.T) {
	if !IsULAIP(net.ParseIP("fc00::1")) {
		t.Error("fc00::1 should be ULA")
	}
	if !IsULAIP(net.ParseIP("fd12:3456:789a::1")) {
		t.Error("fd12:3456:789a::1 should be ULA")
	}
	if IsULAIP(net.ParseIP("::1")) {
		t.Error("::1 should not be ULA")
	}
	if IsULAIP(net.ParseIP("::")) {
		t.Error(":: should not be ULA")
	}
	if IsULAIP(net.ParseIP("2001:4860:4860::8888")) {
		t.Error("public IPv6 should not be ULA")
	}
	if IsULAIP(net.ParseIP("192.168.1.1")) {
		t.Error("IPv4 should not be ULA")
	}
}

func TestValidateBindHostULA(t *testing.T) {
	// ULA should be accepted as private
	for _, addr := range []string{"fc00::1", "fd00::1", "fd12:3456:789a::1"} {
		if err := ValidateBindHost(addr); err != nil {
			t.Errorf("ValidateBindHost ULA %q should pass: %v", addr, err)
		}
		// With port (bracketed)
		bracketed := "[" + addr + "]:9090"
		if err := ValidateBindHost(bracketed); err != nil {
			t.Errorf("ValidateBindHost ULA bracketed %q should pass: %v", bracketed, err)
		}
	}
	// Public IPv6 should be rejected
	if err := ValidateBindHost("2001:4860:4860::8888"); err == nil {
		t.Error("public IPv6 should be rejected")
	}
	if err := ValidateBindHost("[2001:4860:4860::8888]:9090"); err == nil {
		t.Error("public IPv6 with port should be rejected")
	}
	// Unspecified IPv6
	if err := ValidateBindHost("::"); err == nil {
		t.Error("unspecified :: should be rejected")
	}
	if err := ValidateBindHost("[::]:9090"); err == nil {
		t.Error("[::]:9090 should be rejected")
	}
}
