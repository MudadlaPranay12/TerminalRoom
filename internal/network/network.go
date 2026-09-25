package network

import (
	"fmt"
	"net"
	"strings"
)

type NetworkType int

const (
	NetworkNone NetworkType = iota
	NetworkTailscale
	NetworkPrivate
	NetworkLoopback
)

func (n NetworkType) String() string {
	switch n {
	case NetworkTailscale:
		return "tailscale"
	case NetworkPrivate:
		return "private"
	case NetworkLoopback:
		return "loopback"
	default:
		return "none"
	}
}

type PrivateAddress struct {
	InterfaceName  string
	InterfaceIndex int
	Address        string
	NetworkType    NetworkType
}

type Diagnostic struct {
	Available   bool
	Address     string
	Interface   string
	NetworkType string
	Error       string
}

func (d Diagnostic) String() string {
	if d.Error != "" {
		return fmt.Sprintf("Private network detected: NO\nError: %s", d.Error)
	}
	if !d.Available {
		return "Private network detected: NO\nNo usable private network address found."
	}
	return fmt.Sprintf("Private network detected: YES\nPrivate address: %s\nPrivate interface: %s\nNetwork type: %s", d.Address, d.Interface, d.NetworkType)
}

func isTailscaleIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}
	return false
}

func isLoopback(ip net.IP) bool {
	return ip.IsLoopback()
}

func isULAIP(ip net.IP) bool {
	if ip.To4() != nil {
		return false
	}
	if len(ip) == 0 {
		return false
	}
	// ULA fc00::/7 => first byte 0xfc or 0xfd (covers fd00::/8)
	// net.ParseIP returns 16-byte slice for IPv6, 4-byte for IPv4 (already excluded)
	// For IPv6, check first byte
	return ip[0] == 0xfc || ip[0] == 0xfd
}

func classifyAddress(ip net.IP) NetworkType {
	if isTailscaleIP(ip) {
		return NetworkTailscale
	}
	if isPrivateIP(ip) {
		return NetworkPrivate
	}
	if isULAIP(ip) {
		return NetworkPrivate
	}
	if isLoopback(ip) {
		return NetworkLoopback
	}
	return NetworkNone
}

// Exported wrappers for shared classification (used by tlsutil to avoid duplication).

func IsTailscaleIP(ip net.IP) bool { return isTailscaleIP(ip) }
func IsPrivateIP(ip net.IP) bool   { return isPrivateIP(ip) }
func IsLoopbackIP(ip net.IP) bool  { return isLoopback(ip) }
func IsULAIP(ip net.IP) bool       { return isULAIP(ip) }
func ClassifyAddress(ip net.IP) NetworkType {
	return classifyAddress(ip)
}

// ListPrivateAddresses returns all non-loopback private/Tailscale IPs found on up interfaces.
// It excludes public, down, or loopback interfaces. Used for TLS SAN generation.
func ListPrivateAddresses() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate network interfaces: %w", err)
	}
	seen := make(map[string]struct{})
	var ips []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			nt := classifyAddress(ip)
			if nt != NetworkTailscale && nt != NetworkPrivate {
				continue
			}
			key := ip.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			// Copy IP to avoid aliasing
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
	}
	return ips, nil
}

func DiscoverPrivateAddress() (*PrivateAddress, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate network interfaces: %w", err)
	}

	var best *PrivateAddress

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			netType := classifyAddress(ip)
			if netType == NetworkNone {
				continue
			}

			pa := &PrivateAddress{
				InterfaceName:  iface.Name,
				InterfaceIndex: iface.Index,
				Address:        ip.String(),
				NetworkType:    netType,
			}

			if best == nil {
				best = pa
				continue
			}

			if netType == NetworkTailscale && best.NetworkType != NetworkTailscale {
				best = pa
				continue
			}

			if netType == NetworkTailscale && best.NetworkType == NetworkTailscale {
				if pa.Address < best.Address {
					best = pa
				}
			}
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no usable private network address found")
	}

	return best, nil
}

func ValidateBindHost(addr string) error {
	return validateBindHost(addr)
}

func validateBindHost(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	host := addr
	// Extract host if addr is host:port. Use SplitHostPort when it parses;
	// otherwise keep original (avoids misclassifying plain IP without port).
	if strings.Contains(addr, ":") {
		if h, _, err := net.SplitHostPort(addr); err == nil {
			host = h
		} else {
			// If SplitHostPort fails but string contains ":", it may be
			// an IPv6 literal without brackets or malformed. For IPv4 host:port
			// we already handled the successful case. For bare IPv4 like
			// "192.168.1.10" no colon, this branch not taken.
			// Keep host as-is for further classification which will reject.
			// Special handling: try to see if addr itself is a valid IP before rejecting
			// e.g., "8.8.8.8:9090:9090" double port -> treat as malformed -> reject
			if net.ParseIP(strings.TrimSpace(addr)) == nil && !strings.EqualFold(strings.TrimSpace(addr), "localhost") {
				// Malformed host:port should not be silently treated as private.
				return fmt.Errorf("server address %s is not a recognized private address; TerminalRoom requires a private network address (10.x, 172.16-31.x, 192.168.x, 100.64-127.x) or localhost", addr)
			}
		}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("server address is empty; TerminalRoom requires a private network address")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if host == "0.0.0.0" {
		return fmt.Errorf("bind address 0.0.0.0 is not allowed: use a private address or 127.0.0.1 for development")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsUnspecified() {
			return fmt.Errorf("bind address 0.0.0.0 is not allowed: use a private address or 127.0.0.1 for development")
		}
		if ip.IsLoopback() {
			return nil
		}
		nt := ClassifyAddress(ip)
		if nt == NetworkTailscale || nt == NetworkPrivate || nt == NetworkLoopback {
			return nil
		}
		return fmt.Errorf("server address %s is public; TerminalRoom requires a private network address", host)
	}
	// Non-IP hostname that is not localhost: cannot safely classify -> reject
	return fmt.Errorf("server address %s is not a recognized private address; TerminalRoom requires a private network address (10.x, 172.16-31.x, 192.168.x, 100.64-127.x) or localhost", host)
}

func ResolveBindAddress(explicitAddr string, port string) (string, error) {
	explicitAddr = strings.TrimSpace(explicitAddr)
	if explicitAddr != "" {
		if err := validateBindHost(explicitAddr); err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(explicitAddr)
		if trimmed == "localhost" || trimmed == "127.0.0.1" {
			return "127.0.0.1:" + port, nil
		}
		// If explicitAddr already contains a valid host:port, preserve it (trimmed)
		if strings.Contains(trimmed, ":") {
			if _, _, err := net.SplitHostPort(trimmed); err == nil {
				return trimmed, nil
			}
		}
		return trimmed + ":" + port, nil
	}

	pa, err := DiscoverPrivateAddress()
	if err != nil {
		return "", fmt.Errorf("no private network address available: %w", err)
	}

	return pa.Address + ":" + port, nil
}

func ResolveEndpointAddress(explicitAddr string, port string) (string, error) {
	if explicitAddr != "" {
		if explicitAddr == "0.0.0.0" {
			pa, err := DiscoverPrivateAddress()
			if err != nil {
				return "localhost:" + port, nil
			}
			return pa.Address + ":" + port, nil
		}
		if explicitAddr == "localhost" || explicitAddr == "127.0.0.1" {
			return "127.0.0.1:" + port, nil
		}
		return explicitAddr + ":" + port, nil
	}

	pa, err := DiscoverPrivateAddress()
	if err != nil {
		return "", fmt.Errorf("private network unavailable: %w", err)
	}

	return pa.Address + ":" + port, nil
}

func ParseEndpoint(endpoint string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(endpoint)
	if err != nil {
		if strings.Contains(endpoint, ":") {
			return "", "", fmt.Errorf("invalid endpoint format: %s", endpoint)
		}
		return endpoint, "9090", nil
	}
	return host, port, nil
}
