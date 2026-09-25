package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/terminalroom/terminalroom/internal/network"
)

const (
	CertFile = "server.crt"
	KeyFile  = "server.key"
	// CertValidity is the lifetime of a newly generated self-signed certificate.
	// 30 days is short-lived for this temporary/private application (vs. prior 365 days),
	// limiting exposure if a key is compromised while avoiding excessive regeneration.
	CertValidity = 30 * 24 * time.Hour
)

// EnsureCert loads an existing certificate or generates a new self-signed one.
// It is idempotent: existing cert is reused if it is valid and contains all
// required SANs (localhost + loopback + discovered private IPs). Otherwise it
// regenerates the key/certificate atomically.
func EnsureCert(dir string) (tls.Certificate, error) {
	dns, ips := requiredSANs()
	return EnsureCertWithSANs(dir, dns, ips)
}

// EnsureCertWithSANs is like EnsureCert but with explicit required SANs.
// Exported for deterministic testing (use deterministic IPs like 100.64.0.10).
func EnsureCertWithSANs(dir string, requiredDNS []string, requiredIPs []net.IP) (tls.Certificate, error) {
	certPath := filepath.Join(dir, CertFile)
	keyPath := filepath.Join(dir, KeyFile)

	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			loaded, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err == nil {
				// Parse leaf if needed
				if loaded.Leaf == nil && len(loaded.Certificate) > 0 {
					if leaf, err := x509.ParseCertificate(loaded.Certificate[0]); err == nil {
						loaded.Leaf = leaf
					}
				}
				if loaded.Leaf != nil && !needsRegeneration(loaded.Leaf, requiredDNS, requiredIPs) {
					return loaded, nil
				}
				// Needs regeneration — fall through
			}
		}
	}

	return generateSelfSignedWithSANs(dir, requiredDNS, requiredIPs)
}

func requiredSANs() (dns []string, ips []net.IP) {
	dns = []string{"localhost"}
	ips = []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("::1"),
	}

	// Prefer ListPrivateAddresses (all private/tailscale). Fallback to single best.
	var discovered []net.IP
	if list, err := network.ListPrivateAddresses(); err == nil && len(list) > 0 {
		discovered = list
	} else if pa, err := network.DiscoverPrivateAddress(); err == nil {
		if ip := net.ParseIP(pa.Address); ip != nil {
			discovered = []net.IP{ip}
		}
	}

	for _, ip := range discovered {
		if ip == nil {
			continue
		}
		if ipExists(ips, ip) {
			continue
		}
		// Copy
		if ip4 := ip.To4(); ip4 != nil {
			cp := make(net.IP, len(ip4))
			copy(cp, ip4)
			ips = append(ips, cp)
		} else {
			cp := make(net.IP, len(ip))
			copy(cp, ip)
			ips = append(ips, cp)
		}
	}

	return dns, ips
}

func ipExists(list []net.IP, ip net.IP) bool {
	for _, a := range list {
		if a.Equal(ip) {
			return true
		}
	}
	return false
}

func needsRegeneration(cert *x509.Certificate, requiredDNS []string, requiredIPs []net.IP) bool {
	if cert == nil {
		return true
	}
	now := time.Now()
	if now.After(cert.NotAfter) || now.Before(cert.NotBefore) {
		return true
	}
	// Check DNS
	for _, rdns := range requiredDNS {
		found := false
		for _, cdns := range cert.DNSNames {
			if cdns == rdns {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	// Check IPs
	for _, rip := range requiredIPs {
		found := false
		for _, cip := range cert.IPAddresses {
			if cip.Equal(rip) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// NeedsRegeneration is exported for tests.
func NeedsRegeneration(cert *x509.Certificate, requiredDNS []string, requiredIPs []net.IP) bool {
	return needsRegeneration(cert, requiredDNS, requiredIPs)
}

func generateSelfSigned(dir string) (tls.Certificate, error) {
	dns, ips := requiredSANs()
	return generateSelfSignedWithSANs(dir, dns, ips)
}

func generateSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	if serial.Sign() == 0 {
		serial = big.NewInt(1)
	}
	return serial, nil
}

func generateSelfSignedWithSANs(dir string, dnsNames []string, ipAddrs []net.IP) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate key: %w", err)
	}

	serial, err := generateSerial()
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"TerminalRoom"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(CertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create cert directory: %w", err)
	}

	certPath := filepath.Join(dir, CertFile)
	keyPath := filepath.Join(dir, KeyFile)

	// Atomic write via temp files
	certTmp := certPath + ".tmp"
	keyTmp := keyPath + ".tmp"

	certFile, err := os.OpenFile(certTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create cert file: %w", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		certFile.Close()
		return tls.Certificate{}, fmt.Errorf("failed to encode cert: %w", err)
	}
	certFile.Close()

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		os.Remove(certTmp)
		return tls.Certificate{}, fmt.Errorf("failed to marshal key: %w", err)
	}

	keyFile, err := os.OpenFile(keyTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		os.Remove(certTmp)
		return tls.Certificate{}, fmt.Errorf("failed to create key file: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		keyFile.Close()
		os.Remove(certTmp)
		os.Remove(keyTmp)
		return tls.Certificate{}, fmt.Errorf("failed to encode key: %w", err)
	}
	keyFile.Close()

	// Rename atomically
	if err := os.Rename(certTmp, certPath); err != nil {
		os.Remove(certTmp)
		os.Remove(keyTmp)
		return tls.Certificate{}, fmt.Errorf("failed to install cert: %w", err)
	}
	if err := os.Rename(keyTmp, keyPath); err != nil {
		os.Remove(keyTmp)
		return tls.Certificate{}, fmt.Errorf("failed to install key: %w", err)
	}

	// Ensure key permission is 0600 (rename preserves tmp perms, but be explicit)
	_ = os.Chmod(keyPath, 0600)
	_ = os.Chmod(certPath, 0600)

	return tls.LoadX509KeyPair(certPath, keyPath)
}

// GenerateSelfSignedWithIPs is a test helper to generate with explicit IPs.
// DNSNames is always ["localhost"] plus loopback IPs are added if not present.
func GenerateSelfSignedWithIPs(dir string, ips []net.IP) (tls.Certificate, error) {
	dns := []string{"localhost"}
	// Ensure loopback are included
	base := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	for _, ip := range ips {
		if !ipExists(base, ip) {
			base = append(base, ip)
		}
	}
	return generateSelfSignedWithSANs(dir, dns, base)
}

// ServerConfig returns a TLS configuration for the server.
// It loads or generates a self-signed certificate covering localhost + private IPs.
func ServerConfig(certDir string) (*tls.Config, error) {
	cert, err := EnsureCert(certDir)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientConfig returns a TLS configuration for the client.
// It pins the server certificate from certDir for trust.
//
// Trust model: The client trusts only the specific self-signed certificate
// found in the certificate directory. Both server and client must share
// the same certificate directory or the certificate must be distributed
// to the client separately.
func ClientConfig(certDir string) (*tls.Config, error) {
	certPath := filepath.Join(certDir, CertFile)
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read server certificate from %s: %w", certPath, err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse server certificate")
	}

	return &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}, nil
}
