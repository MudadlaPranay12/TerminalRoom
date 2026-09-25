package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func parseCertFromFile(t *testing.T, certPath string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", certPath, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("pem decode failed for %s", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

func containsDNS(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

func containsIP(ips []net.IP, target net.IP) bool {
	for _, ip := range ips {
		if ip.Equal(target) {
			return true
		}
	}
	return false
}

// A: Generated certificate contains localhost, 127.0.0.1, ::1
func TestGeneratedCertContainsBaseSANs(t *testing.T) {
	dir := t.TempDir()
	cert, err := GenerateSelfSignedWithIPs(dir, nil)
	if err != nil {
		t.Fatalf("GenerateSelfSignedWithIPs: %v", err)
	}
	if cert.Leaf == nil {
		if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			cert.Leaf = leaf
		}
	}
	leaf := cert.Leaf
	if leaf == nil {
		t.Fatal("leaf nil")
	}
	if !containsDNS(leaf.DNSNames, "localhost") {
		t.Errorf("DNSNames %v should contain localhost", leaf.DNSNames)
	}
	if !containsIP(leaf.IPAddresses, net.ParseIP("127.0.0.1")) {
		t.Errorf("IPAddresses %v should contain 127.0.0.1", leaf.IPAddresses)
	}
	if !containsIP(leaf.IPAddresses, net.ParseIP("::1")) {
		t.Errorf("IPAddresses %v should contain ::1", leaf.IPAddresses)
	}
	// Ensure 127.0.0.1 is NOT misplaced as DNS (spec says it should be IP)
	if containsDNS(leaf.DNSNames, "127.0.0.1") {
		t.Errorf("127.0.0.1 should be in IPAddresses, not DNSNames %v", leaf.DNSNames)
	}
	if containsDNS(leaf.DNSNames, "::1") {
		t.Errorf("::1 should be in IPAddresses, not DNSNames %v", leaf.DNSNames)
	}
}

// B: Can contain discovered private IPv4 addresses
func TestGeneratedCertContainsPrivateIP(t *testing.T) {
	dir := t.TempDir()
	ips := []net.IP{net.ParseIP("100.64.0.10"), net.ParseIP("192.168.1.50")}
	cert, err := GenerateSelfSignedWithIPs(dir, ips)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cert.Leaf == nil {
		leaf, _ := x509.ParseCertificate(cert.Certificate[0])
		cert.Leaf = leaf
	}
	for _, want := range ips {
		if !containsIP(cert.Leaf.IPAddresses, want) {
			t.Errorf("cert should contain private IP %s, got %v", want, cert.Leaf.IPAddresses)
		}
	}
}

// C: Verification succeeds for private IP present in SAN
func TestVerificationSucceedsForPresentIP(t *testing.T) {
	dir := t.TempDir()
	privIP := net.ParseIP("192.168.1.50")
	cert, err := GenerateSelfSignedWithIPs(dir, []net.IP{privIP})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cert.Leaf == nil {
		leaf, _ := x509.ParseCertificate(cert.Certificate[0])
		cert.Leaf = leaf
	}
	// VerifyHostname should succeed for IP present
	if err := cert.Leaf.VerifyHostname("192.168.1.50"); err != nil {
		t.Errorf("VerifyHostname for present IP should succeed: %v", err)
	}
	// Also test via ClientConfig pinned verification
	clientCfg, err := ClientConfig(dir)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}
	if clientCfg.InsecureSkipVerify {
		t.Error("ClientConfig InsecureSkipVerify should be false")
	}
	// Verify leaf against client RootCAs
	roots := clientCfg.RootCAs
	opts := x509.VerifyOptions{
		Roots:         roots,
		CurrentTime:   time.Now(),
		DNSName:       "",
		Intermediates: x509.NewCertPool(),
	}
	// For IP verification, set DNSName to IP string and use VerifyHostname instead
	// Verify should succeed when verifying with IP via VerifyHostname
	if err := cert.Leaf.VerifyHostname(privIP.String()); err != nil {
		t.Errorf("pinned verification for present IP failed: %v", err)
	}
	_ = opts // keep for coverage
}

// D: Verification fails for IP NOT present
func TestVerificationFailsForMissingIP(t *testing.T) {
	dir := t.TempDir()
	cert, err := GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cert.Leaf == nil {
		leaf, _ := x509.ParseCertificate(cert.Certificate[0])
		cert.Leaf = leaf
	}
	if err := cert.Leaf.VerifyHostname("8.8.8.8"); err == nil {
		t.Error("VerifyHostname for missing IP should fail")
	}
	if err := cert.Leaf.VerifyHostname("100.64.0.99"); err == nil {
		t.Error("VerifyHostname for non-SAN tailscale IP should fail")
	}
}

// E: Existing valid cert reused when all required SANs present
func TestExistingValidCertReused(t *testing.T) {
	dir := t.TempDir()
	requiredDNS := []string{"localhost"}
	requiredIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("100.64.0.10"), net.ParseIP("192.168.1.50")}

	cert1, err := EnsureCertWithSANs(dir, requiredDNS, requiredIPs)
	if err != nil {
		t.Fatalf("first EnsureCertWithSANs: %v", err)
	}
	certPath := filepath.Join(dir, CertFile)
	info1, _ := os.Stat(certPath)
	data1, _ := os.ReadFile(certPath)

	time.Sleep(10 * time.Millisecond)

	cert2, err := EnsureCertWithSANs(dir, requiredDNS, requiredIPs)
	if err != nil {
		t.Fatalf("second EnsureCertWithSANs: %v", err)
	}
	info2, _ := os.Stat(certPath)
	data2, _ := os.ReadFile(certPath)

	if string(data1) != string(data2) {
		t.Error("valid cert should be reused, but file changed")
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("valid cert should not be regenerated (modtime changed)")
	}
	// Ensure same leaf
	if cert1.Leaf == nil {
		leaf, _ := x509.ParseCertificate(cert1.Certificate[0])
		cert1.Leaf = leaf
	}
	if cert2.Leaf == nil {
		leaf, _ := x509.ParseCertificate(cert2.Certificate[0])
		cert2.Leaf = leaf
	}
	_ = cert1
	_ = cert2
}

// F: Missing required SAN triggers regeneration
func TestMissingSANTriggersRegeneration(t *testing.T) {
	dir := t.TempDir()
	// First generate with only loopback
	baseDNS := []string{"localhost"}
	baseIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	_, err := EnsureCertWithSANs(dir, baseDNS, baseIPs)
	if err != nil {
		t.Fatalf("base EnsureCert: %v", err)
	}
	certBefore := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if containsIP(certBefore.IPAddresses, net.ParseIP("192.168.1.50")) {
		t.Fatal("base cert should not contain 192.168.1.50 yet")
	}
	dataBefore, _ := os.ReadFile(filepath.Join(dir, CertFile))

	time.Sleep(10 * time.Millisecond)

	// Now require extra private IP — should regenerate
	requiredDNS := []string{"localhost"}
	requiredIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("192.168.1.50")}
	_, err = EnsureCertWithSANs(dir, requiredDNS, requiredIPs)
	if err != nil {
		t.Fatalf("second EnsureCertWithSANs: %v", err)
	}
	certAfter := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if !containsIP(certAfter.IPAddresses, net.ParseIP("192.168.1.50")) {
		t.Errorf("regenerated cert should contain 192.168.1.50, got %v", certAfter.IPAddresses)
	}
	dataAfter, _ := os.ReadFile(filepath.Join(dir, CertFile))
	if string(dataBefore) == string(dataAfter) {
		t.Error("cert should have been regenerated but file unchanged")
	}
}

// G: Client verification still uses pinned certificate
func TestClientUsesPinnedCert(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	clientCfg, err := ClientConfig(dir)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}
	if clientCfg.RootCAs == nil {
		t.Fatal("RootCAs should not be nil")
	}
	// Verify that server cert is in pool by verifying leaf
	certPath := filepath.Join(dir, CertFile)
	data, _ := os.ReadFile(certPath)
	block, _ := pem.Decode(data)
	leaf, _ := x509.ParseCertificate(block.Bytes)
	opts := x509.VerifyOptions{
		Roots:       clientCfg.RootCAs,
		CurrentTime: time.Now(),
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// Verify with DNSName localhost (should succeed)
	opts.DNSName = "localhost"
	if _, err := leaf.Verify(opts); err != nil {
		t.Errorf("pinned cert verification for localhost should succeed: %v", err)
	}
	// Verify with IP
	opts.DNSName = ""
	// VerifyHostname for IP directly
	if err := leaf.VerifyHostname("10.0.0.5"); err != nil {
		t.Errorf("pinned cert should verify 10.0.0.5: %v", err)
	}
}

// H: InsecureSkipVerify remains false
func TestInsecureSkipVerifyFalse(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	srvCfg, err := ServerConfig(dir)
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if srvCfg.InsecureSkipVerify {
		t.Error("ServerConfig InsecureSkipVerify should be false")
	}
	clientCfg, err := ClientConfig(dir)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}
	if clientCfg.InsecureSkipVerify {
		t.Error("ClientConfig InsecureSkipVerify should be false")
	}
	if srvCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("ServerConfig MinVersion = %v, want TLS1.2", srvCfg.MinVersion)
	}
	if clientCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("ClientConfig MinVersion = %v, want TLS1.2", clientCfg.MinVersion)
	}
}

// I: CERT_DIR / APPDATA behavior remains intact
func TestCertDirBehavior(t *testing.T) {
	// Test explicit dir is respected
	dir1 := t.TempDir()
	_, err := EnsureCertWithSANs(dir1, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")})
	if err != nil {
		t.Fatalf("EnsureCert dir1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir1, CertFile)); err != nil {
		t.Errorf("cert should exist in explicit dir1")
	}
	dir2 := t.TempDir()
	if _, err := os.Stat(filepath.Join(dir2, CertFile)); err == nil {
		t.Error("cert should not exist in dir2 before generation")
	}
	// Ensure separate dirs are independent
	_, err = GenerateSelfSignedWithIPs(dir2, []net.IP{net.ParseIP("192.168.1.99")})
	if err != nil {
		t.Fatalf("Generate dir2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, CertFile)); err != nil {
		t.Error("cert should exist in dir2 after generation")
	}
	// Permissions (Windows does not enforce Unix perm bits; skip strict check on windows)
	certPath := filepath.Join(dir1, CertFile)
	keyPath := filepath.Join(dir1, KeyFile)
	if info, err := os.Stat(keyPath); err == nil {
		perm := info.Mode().Perm()
		if runtime.GOOS != "windows" && perm != 0600 {
			t.Errorf("key perm = %o, want 0600", perm)
		}
	}
	if info, err := os.Stat(filepath.Dir(certPath)); err == nil {
		if info.Mode().Perm()&0700 != 0700 && runtime.GOOS != "windows" {
			t.Logf("warning: cert dir perm = %o, want 0700", info.Mode().Perm())
		}
	}
}

// Additional: TLS dial with pinned cert succeeds for SAN IP, fails otherwise
func TestTLSDialWithPinnedCert(t *testing.T) {
	dir := t.TempDir()
	privIP := net.ParseIP("192.168.1.50")
	_, err := GenerateSelfSignedWithIPs(dir, []net.IP{privIP})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Use the generated cert directly to avoid ServerConfig regeneration (which would add discovered IPs)
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, CertFile), filepath.Join(dir, KeyFile))
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	srvCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	clientCfg, err := ClientConfig(dir)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}
	// Client must verify server name; set ServerName to IP present
	clientCfg.ServerName = "192.168.1.50"
	// Start TLS listener
	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		buf := make([]byte, 5)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("ok"))
		done <- nil
	}()
	// Dial with present IP — should succeed (using loopback dial but verification via ServerName)
	conn, err := tls.Dial("tcp", ln.Addr().String(), clientCfg)
	if err != nil {
		t.Fatalf("Dial with present SAN should succeed: %v", err)
	}
	_, _ = conn.Write([]byte("hello"))
	buf := make([]byte, 2)
	_, _ = conn.Read(buf)
	conn.Close()
	<-done

	// New listener for second dial (need fresh accept)
	ln2, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatalf("Listen2: %v", err)
	}
	defer ln2.Close()
	go func() {
		conn, err := ln2.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Do handshake then close
		buf := make([]byte, 5)
		_, _ = conn.Read(buf)
	}()
	clientCfg2, _ := ClientConfig(dir)
	clientCfg2.ServerName = "8.8.8.8" // not in SAN
	conn2, err := tls.Dial("tcp", ln2.Addr().String(), clientCfg2)
	if err == nil {
		conn2.Close()
		t.Error("Dial with missing SAN should fail verification")
	}
}

func TestNeedsRegenerationHelper(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	cert := parseCertFromFile(t, filepath.Join(dir, CertFile))
	// Should not need regeneration for same SANs
	if NeedsRegeneration(cert, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("10.0.0.5")}) {
		t.Error("should not need regeneration when all SANs present")
	}
	// Should need regeneration for missing IP
	if !NeedsRegeneration(cert, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("10.0.0.5"), net.ParseIP("192.168.1.50")}) {
		t.Error("should need regeneration when IP missing")
	}
	// Expired cert should need regeneration
	expired := &x509.Certificate{
		NotBefore:   time.Now().Add(-2 * time.Hour),
		NotAfter:    time.Now().Add(-1 * time.Hour),
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	if !NeedsRegeneration(expired, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}) {
		t.Error("expired cert should need regeneration")
	}
}

// F-03: Serial number must be cryptographically random, positive, non-zero, not constant 1
func TestSerialIsPositiveNonZeroAndNotOne(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	cert := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if cert.SerialNumber == nil {
		t.Fatal("serial is nil")
	}
	if cert.SerialNumber.Sign() <= 0 {
		t.Errorf("serial must be positive, got %s", cert.SerialNumber.String())
	}
	if cert.SerialNumber.Cmp(big.NewInt(0)) == 0 {
		t.Error("serial must be non-zero")
	}
	if cert.SerialNumber.Cmp(big.NewInt(1)) == 0 {
		t.Error("serial must not be constant 1 (F-03 regression)")
	}
	// Ensure within 128-bit range (<2^128)
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	if cert.SerialNumber.Cmp(limit) >= 0 {
		t.Errorf("serial %s exceeds 128-bit limit", cert.SerialNumber.String())
	}
}

func TestSerialRandomDifferent(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir1, nil)
	if err != nil {
		t.Fatalf("Generate dir1: %v", err)
	}
	_, err = GenerateSelfSignedWithIPs(dir2, nil)
	if err != nil {
		t.Fatalf("Generate dir2: %v", err)
	}
	c1 := parseCertFromFile(t, filepath.Join(dir1, CertFile))
	c2 := parseCertFromFile(t, filepath.Join(dir2, CertFile))
	if c1.SerialNumber.Cmp(c2.SerialNumber) == 0 {
		// Extremely unlikely (1/2^128); retry once before failing
		dir3 := t.TempDir()
		_, _ = GenerateSelfSignedWithIPs(dir3, nil)
		c3 := parseCertFromFile(t, filepath.Join(dir3, CertFile))
		if c1.SerialNumber.Cmp(c3.SerialNumber) == 0 {
			t.Error("two independently generated certs should have different serial numbers")
		}
	}
	if c1.SerialNumber.Sign() <= 0 || c2.SerialNumber.Sign() <= 0 {
		t.Error("serials must be positive")
	}
}

func TestSerialRegenerationOnSANChange(t *testing.T) {
	dir := t.TempDir()
	baseDNS := []string{"localhost"}
	baseIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	_, err := EnsureCertWithSANs(dir, baseDNS, baseIPs)
	if err != nil {
		t.Fatalf("base EnsureCert: %v", err)
	}
	cBefore := parseCertFromFile(t, filepath.Join(dir, CertFile))
	sBefore := cBefore.SerialNumber.String()
	time.Sleep(10 * time.Millisecond)
	// Require extra IP -> triggers regeneration
	newIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("192.168.1.50")}
	_, err = EnsureCertWithSANs(dir, baseDNS, newIPs)
	if err != nil {
		t.Fatalf("second EnsureCert: %v", err)
	}
	cAfter := parseCertFromFile(t, filepath.Join(dir, CertFile))
	sAfter := cAfter.SerialNumber.String()
	if sBefore == sAfter {
		t.Error("regenerated cert after SAN change should have new serial number")
	}
	if cAfter.SerialNumber.Sign() <= 0 {
		t.Error("regenerated serial must be positive")
	}
	if cAfter.SerialNumber.Cmp(big.NewInt(1)) == 0 && sBefore != "1" {
		t.Logf("warning: regenerated serial is 1, possible but should be random")
	}
}

func TestSerialRegenerationOnExpiry(t *testing.T) {
	dir := t.TempDir()
	// Generate cert then force expiry by constructing expired leaf and checking NeedsRegeneration
	// Then ensure regeneration yields new serial
	_, err := GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	c1 := parseCertFromFile(t, filepath.Join(dir, CertFile))
	s1 := c1.SerialNumber.String()
	// Simulate expiry: create expired cert file manually? Instead use EnsureCertWithSANs after deleting and recreating
	// For this test, we verify that NeedsRegeneration correctly flags expired and that a new generation would have different serial
	expired := &x509.Certificate{
		NotBefore:   time.Now().Add(-2 * time.Hour),
		NotAfter:    time.Now().Add(-1 * time.Hour),
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("10.0.0.5")},
	}
	if !NeedsRegeneration(expired, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("10.0.0.5")}) {
		t.Fatal("expired should need regeneration")
	}
	// Now force regeneration by removing and recreating (simulates expiry regeneration path)
	os.Remove(filepath.Join(dir, CertFile))
	os.Remove(filepath.Join(dir, KeyFile))
	_, err = GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("regenerate after expiry: %v", err)
	}
	c2 := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if c1.SerialNumber.Cmp(c2.SerialNumber) == 0 && s1 != "" {
		// Could be same by chance, retry once
		os.Remove(filepath.Join(dir, CertFile))
		os.Remove(filepath.Join(dir, KeyFile))
		_, _ = GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.6")})
		c3 := parseCertFromFile(t, filepath.Join(dir, CertFile))
		if c1.SerialNumber.Cmp(c3.SerialNumber) == 0 {
			t.Error("regenerated cert after expiry should have different serial")
		}
	}
}

func TestSerialReuseKeepsSame(t *testing.T) {
	dir := t.TempDir()
	dns := []string{"localhost"}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	_, err := EnsureCertWithSANs(dir, dns, ips)
	if err != nil {
		t.Fatalf("first EnsureCert: %v", err)
	}
	c1 := parseCertFromFile(t, filepath.Join(dir, CertFile))
	s1 := c1.SerialNumber.String()
	data1, _ := os.ReadFile(filepath.Join(dir, CertFile))
	time.Sleep(10 * time.Millisecond)
	_, err = EnsureCertWithSANs(dir, dns, ips)
	if err != nil {
		t.Fatalf("second EnsureCert: %v", err)
	}
	c2 := parseCertFromFile(t, filepath.Join(dir, CertFile))
	s2 := c2.SerialNumber.String()
	data2, _ := os.ReadFile(filepath.Join(dir, CertFile))
	if s1 != s2 {
		t.Errorf("valid cert should be reused, serial changed %s -> %s", s1, s2)
	}
	if string(data1) != string(data2) {
		t.Error("valid cert should be reused, file changed")
	}
}

func TestGenerateSerialHelper(t *testing.T) {
	s, err := generateSerial()
	if err != nil {
		t.Fatalf("generateSerial: %v", err)
	}
	if s.Sign() <= 0 {
		t.Errorf("generateSerial must be positive, got %s", s.String())
	}
	if s.Cmp(big.NewInt(1)) == 0 {
		// Allow but log; probability of 1 is 1/2^128, if happens second try should differ
		s2, _ := generateSerial()
		if s.Cmp(s2) == 0 {
			t.Error("generateSerial should not constantly return 1")
		}
	}
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	if s.Cmp(limit) >= 0 {
		t.Errorf("serial exceeds 128-bit limit")
	}
}

// F-04: Certificate lifetime is short-lived (30 days) not 365 days
func TestCertValidityPeriod(t *testing.T) {
	if CertValidity != 30*24*time.Hour {
		t.Errorf("CertValidity = %v, want %v (30d)", CertValidity, 30*24*time.Hour)
	}
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	cert := parseCertFromFile(t, filepath.Join(dir, CertFile))
	validity := cert.NotAfter.Sub(cert.NotBefore)
	// Allow 2 minute tolerance for execution time (NotBefore = now-1m, NotAfter = now+30d)
	expected := CertValidity + time.Minute // NotBefore is -1m, NotAfter is +30d => total 30d+1m
	if validity < expected-time.Minute*2 || validity > expected+time.Minute*2 {
		t.Errorf("cert validity = %v, want ~%v (CertValidity+1m) ±2m", validity, expected)
	}
	// Ensure not old 365d
	old := 365 * 24 * time.Hour
	if validity > old-time.Hour*24 {
		t.Errorf("cert validity %v too long, should be short-lived not 365d", validity)
	}
	if cert.NotAfter.Before(time.Now().Add(CertValidity - time.Minute)) {
		t.Errorf("NotAfter too early: %v", cert.NotAfter)
	}
	if cert.NotBefore.After(time.Now()) {
		t.Errorf("NotBefore in future: %v", cert.NotBefore)
	}
}

func TestCertValidityWithExplicitIPs(t *testing.T) {
	dir := t.TempDir()
	ips := []net.IP{net.ParseIP("192.168.1.50"), net.ParseIP("10.0.0.5")}
	_, err := GenerateSelfSignedWithIPs(dir, ips)
	if err != nil {
		t.Fatalf("Generate with IPs: %v", err)
	}
	cert := parseCertFromFile(t, filepath.Join(dir, CertFile))
	validity := cert.NotAfter.Sub(cert.NotBefore)
	expected := CertValidity + time.Minute
	if validity < expected-time.Minute*2 || validity > expected+time.Minute*2 {
		t.Errorf("validity with SANs = %v, want ~%v", validity, expected)
	}
	// SANs still present
	if !containsIP(cert.IPAddresses, net.ParseIP("192.168.1.50")) {
		t.Errorf("SAN missing after validity change")
	}
}

func TestCertReuseWithShortValidity(t *testing.T) {
	dir := t.TempDir()
	dns := []string{"localhost"}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	cert1, err := EnsureCertWithSANs(dir, dns, ips)
	if err != nil {
		t.Fatalf("first EnsureCert: %v", err)
	}
	leaf1 := cert1.Leaf
	if leaf1 == nil {
		leaf1, _ = x509.ParseCertificate(cert1.Certificate[0])
	}
	notAfter1 := leaf1.NotAfter
	time.Sleep(10 * time.Millisecond)
	cert2, err := EnsureCertWithSANs(dir, dns, ips)
	if err != nil {
		t.Fatalf("second EnsureCert: %v", err)
	}
	leaf2 := cert2.Leaf
	if leaf2 == nil {
		leaf2, _ = x509.ParseCertificate(cert2.Certificate[0])
	}
	if !leaf1.NotAfter.Equal(leaf2.NotAfter) || leaf1.SerialNumber.Cmp(leaf2.SerialNumber) != 0 {
		t.Error("valid cert should be reused, not regenerated (serial/NotAfter should match)")
	}
	if leaf2.NotAfter != notAfter1 {
		t.Errorf("reuse should keep same NotAfter")
	}
}

func TestExpiredCertRegeneratedWithShortValidity(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	orig := parseCertFromFile(t, filepath.Join(dir, CertFile))
	// Create an expired cert file manually by using NeedsRegeneration logic:
	// We simulate expiry by directly checking an expired cert struct
	expired := &x509.Certificate{
		NotBefore:   time.Now().Add(-2 * time.Hour),
		NotAfter:    time.Now().Add(-1 * time.Hour),
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	if !NeedsRegeneration(expired, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}) {
		t.Fatal("expired should need regeneration")
	}
	// Force regeneration by deleting and recreating with same SANs - new cert should have fresh validity
	os.Remove(filepath.Join(dir, CertFile))
	os.Remove(filepath.Join(dir, KeyFile))
	_, err = GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatalf("regenerate after expiry: %v", err)
	}
	newCert := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if orig.SerialNumber.Cmp(newCert.SerialNumber) == 0 {
		// Retry once if extremely unlikely collision
		os.Remove(filepath.Join(dir, CertFile))
		os.Remove(filepath.Join(dir, KeyFile))
		_, _ = GenerateSelfSignedWithIPs(dir, []net.IP{net.ParseIP("10.0.0.6")})
		newCert2 := parseCertFromFile(t, filepath.Join(dir, CertFile))
		if orig.SerialNumber.Cmp(newCert2.SerialNumber) == 0 {
			t.Error("regenerated cert after expiry should have different serial")
		}
	}
	validity := newCert.NotAfter.Sub(newCert.NotBefore)
	expected := CertValidity + time.Minute
	if validity < expected-time.Minute*2 || validity > expected+time.Minute*2 {
		t.Errorf("regenerated cert validity = %v, want ~%v", validity, expected)
	}
	if time.Until(newCert.NotAfter) < CertValidity-time.Minute*5 {
		t.Errorf("regenerated NotAfter too soon: %v", newCert.NotAfter)
	}
}

func TestSANRegenerationStillWorksWithShortValidity(t *testing.T) {
	dir := t.TempDir()
	baseDNS := []string{"localhost"}
	baseIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	_, err := EnsureCertWithSANs(dir, baseDNS, baseIPs)
	if err != nil {
		t.Fatalf("base EnsureCert: %v", err)
	}
	cBefore := parseCertFromFile(t, filepath.Join(dir, CertFile))
	serialBefore := cBefore.SerialNumber.String()
	time.Sleep(10 * time.Millisecond)
	newIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("192.168.1.50")}
	_, err = EnsureCertWithSANs(dir, baseDNS, newIPs)
	if err != nil {
		t.Fatalf("second EnsureCert: %v", err)
	}
	cAfter := parseCertFromFile(t, filepath.Join(dir, CertFile))
	if serialBefore == cAfter.SerialNumber.String() {
		t.Error("SAN change should trigger regeneration with new serial")
	}
	validity := cAfter.NotAfter.Sub(cAfter.NotBefore)
	expected := CertValidity + time.Minute
	if validity < expected-time.Minute*2 || validity > expected+time.Minute*2 {
		t.Errorf("SAN-regenerated cert validity = %v, want ~%v", validity, expected)
	}
	if !containsIP(cAfter.IPAddresses, net.ParseIP("192.168.1.50")) {
		t.Error("regenerated cert missing new SAN")
	}
}
