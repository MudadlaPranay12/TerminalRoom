package invitation

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/terminalroom/terminalroom/internal/network"
)

const (
	TokenPrefix   = "TRINV-"
	TokenByteLen  = 24
	DefaultExpiry = 10 * time.Minute
)

type Invitation struct {
	Token     string
	RoomID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	Consumed  bool
	mu        sync.Mutex
}

func (inv *Invitation) IsExpired() bool {
	return time.Now().After(inv.ExpiresAt)
}

func (inv *Invitation) Consume() bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	if inv.Consumed {
		return false
	}
	if inv.IsExpired() {
		return false
	}
	inv.Consumed = true
	return true
}

func (inv *Invitation) IsValid() bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return !inv.Consumed && !inv.IsExpired()
}

type Manager struct {
	mu          sync.RWMutex
	invitations map[string]*Invitation
}

func NewManager() *Manager {
	return &Manager{
		invitations: make(map[string]*Invitation),
	}
}

func GenerateToken() (string, error) {
	b := make([]byte, TokenByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return TokenPrefix + hex.EncodeToString(b), nil
}

func (m *Manager) Create(roomID string, expiry time.Duration) (*Invitation, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	inv := &Invitation{
		Token:     token,
		RoomID:    roomID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(expiry),
		Consumed:  false,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.invitations[token] = inv
	return inv, nil
}

func (m *Manager) Validate(token, roomID string) (*Invitation, error) {
	m.mu.RLock()
	inv, exists := m.invitations[token]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("invalid invitation")
	}

	if inv.IsExpired() {
		return nil, fmt.Errorf("invitation expired")
	}

	if subtle.ConstantTimeCompare([]byte(inv.RoomID), []byte(roomID)) != 1 {
		return nil, fmt.Errorf("invitation does not match this room")
	}

	if !inv.Consume() {
		return nil, fmt.Errorf("invitation already used")
	}

	return inv, nil
}

func (m *Manager) Remove(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.invitations, token)
}

func (m *Manager) RemoveByRoom(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, inv := range m.invitations {
		if inv.RoomID == roomID {
			delete(m.invitations, token)
		}
	}
}

func (m *Manager) ValidateByToken(token string) (*Invitation, error) {
	m.mu.RLock()
	inv, exists := m.invitations[token]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("invalid invitation")
	}

	if inv.IsExpired() {
		return nil, fmt.Errorf("invitation expired")
	}

	if !inv.Consume() {
		return nil, fmt.Errorf("invitation already used")
	}

	return inv, nil
}

func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, inv := range m.invitations {
		if now.After(inv.ExpiresAt) || inv.Consumed {
			delete(m.invitations, token)
		}
	}
}

func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.invitations)
}

// IsValidTokenFormat checks whether a token has the expected TRINV- + 48 hex chars format.
func IsValidTokenFormat(token string) bool {
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}
	hexPart := strings.TrimPrefix(token, TokenPrefix)
	if len(hexPart) != TokenByteLen*2 {
		return false
	}
	// Ensure hex chars
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return false
	}
	return true
}

// ValidateEndpoint validates a bundled endpoint string (host:port).
// It requires host:port, rejects 0.0.0.0, rejects public IPs, allows localhost/loopback/private/tailscale.
func ValidateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}
	if host == "" {
		return fmt.Errorf("invalid endpoint: empty host")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid endpoint port")
	}
	// Reject 0.0.0.0 explicitly
	if host == "0.0.0.0" {
		return fmt.Errorf("endpoint 0.0.0.0 not allowed")
	}
	// Allow localhost
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	// Try IP
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsUnspecified() {
			return fmt.Errorf("endpoint 0.0.0.0 not allowed")
		}
		if ip.IsLoopback() {
			return nil
		}
		nt := network.ClassifyAddress(ip)
		if nt == network.NetworkNone {
			return fmt.Errorf("endpoint must be private network address")
		}
		return nil
	}
	return fmt.Errorf("invalid endpoint host")
}

// ParseInvite parses a raw invitation string which may be bare token or token@endpoint.
// It returns the token, optional endpoint, or error.
// Rules: trim whitespace, accept bare or bundled, validate token, validate endpoint separately.
func ParseInvite(raw string) (token string, endpoint string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", fmt.Errorf("empty invitation")
	}
	if strings.Count(s, "@") > 1 {
		return "", "", fmt.Errorf("malformed invitation: multiple @")
	}
	if strings.Contains(s, "@") {
		parts := strings.SplitN(s, "@", 2)
		tokenPart := strings.TrimSpace(parts[0])
		endpointPart := strings.TrimSpace(parts[1])
		if tokenPart == "" || endpointPart == "" {
			return "", "", fmt.Errorf("malformed invitation")
		}
		if !IsValidTokenFormat(tokenPart) {
			return "", "", fmt.Errorf("invalid token")
		}
		if err := ValidateEndpoint(endpointPart); err != nil {
			return "", "", err
		}
		return tokenPart, endpointPart, nil
	}
	if !IsValidTokenFormat(s) {
		return "", "", fmt.Errorf("invalid token")
	}
	return s, "", nil
}

// BundledInvitation composes a bundled invitation string.
// It validates the endpoint via ValidateEndpoint before bundling; if the endpoint
// is invalid, it returns the bare token to avoid advertising a public/invalid endpoint.
func BundledInvitation(token, endpoint string) string {
	token = strings.TrimSpace(token)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return token
	}
	if err := ValidateEndpoint(endpoint); err != nil {
		// Do not bundle invalid endpoint — return bare token (still valid auth)
		return token
	}
	return token + "@" + endpoint
}
