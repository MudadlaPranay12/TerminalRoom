package launcher

import (
	"fmt"
	"net"
	"strings"

	"github.com/terminalroom/terminalroom/internal/config"
	"github.com/terminalroom/terminalroom/internal/invitation"
	"github.com/terminalroom/terminalroom/internal/network"
	"github.com/terminalroom/terminalroom/internal/server"
)

// Mode represents the CLI mode selected from arguments.
type Mode string

const (
	ModeLauncher Mode = "launcher"
	ModeServer   Mode = "server"
	ModeClient   Mode = "client"
	ModeDiagnose Mode = "diagnose"
	ModeHelp     Mode = "help"
	ModeUnknown  Mode = "unknown"
)

// ResolveMode determines which mode to run based on os.Args style input.
// args is without program name, e.g. os.Args[1:].
// No args => launcher (productized).
func ResolveMode(args []string) Mode {
	if len(args) == 0 {
		return ModeLauncher
	}
	switch args[0] {
	case "server":
		return ModeServer
	case "client":
		return ModeClient
	case "diagnose":
		return ModeDiagnose
	case "--help", "-h", "help":
		return ModeHelp
	default:
		return ModeUnknown
	}
}

// Launcher orchestrates an embedded server and client.
type Launcher struct {
	cfg      *config.Config
	server   *server.Server
	bindAddr string
	endpoint string
}

// New creates a Launcher that has initialized (but not yet started) an embedded server.
// It performs:
// - config loading (caller may pass cfg, or we load)
// - private address resolution
// - 0.0.0.0 rejection unless DEVELOPMENT_MODE
// - server creation using existing server.Server
func New(cfg *config.Config) (*Launcher, error) {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}
	if err := cfg.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("failed to ensure directories: %w", err)
	}

	bindAddr, endpointAddr, fullBindAddr, err := ResolveAddresses(cfg)
	if err != nil {
		return nil, err
	}

	// Reject 0.0.0.0 outside development mode exactly as server mode does.
	if bindAddr == "0.0.0.0" {
		if !cfg.DevelopmentMode {
			return nil, fmt.Errorf("binding to 0.0.0.0 (all interfaces) is not allowed in normal mode: set SERVER_ADDR to a private IP or SERVER_ADDR=localhost for development, or set DEVELOPMENT_MODE=1 to allow 0.0.0.0")
		}
	}

	srv, err := server.NewWithTTLAndEndpoint(fullBindAddr, cfg.CertDir, cfg.RoomTTL, endpointAddr)
	if err != nil {
		// Provide clear user-facing wrapper for port conflict etc.
		msg := err.Error()
		if strings.Contains(msg, "address already in use") || strings.Contains(msg, "bind") || strings.Contains(msg, "Only one usage") {
			return nil, fmt.Errorf("port already in use (%s): %w", fullBindAddr, err)
		}
		return nil, err
	}

	return &Launcher{
		cfg:      cfg,
		server:   srv,
		bindAddr: bindAddr,
		endpoint: srv.Endpoint(),
	}, nil
}

// ResolveAddresses resolves bind and endpoint addresses using existing network layer.
// It mirrors the logic in cmd/terminalroom main.go runServer to guarantee consistency.
func ResolveAddresses(cfg *config.Config) (bindAddr, endpointAddr, fullBindAddr string, err error) {
	port := cfg.PortString()
	bindAddr = strings.TrimSpace(cfg.ServerAddr)
	endpointAddr = strings.TrimSpace(cfg.EndpointAddr)

	// F-01: Reject explicit public SERVER_ADDR in normal operation.
	// Reuse network classification; only 0.0.0.0 is gated by DEVELOPMENT_MODE elsewhere.
	// Preserve DEVELOPMENT_MODE=1 exception for 0.0.0.0 (but not for public IPs).
	if bindAddr != "" {
		trimmedForZero := strings.TrimSpace(bindAddr)
		isZero := trimmedForZero == "0.0.0.0"
		if !isZero && strings.Contains(trimmedForZero, ":") {
			if h, _, err := net.SplitHostPort(trimmedForZero); err == nil && h == "0.0.0.0" {
				isZero = true
			} else if strings.TrimSpace(trimmedForZero) == "[::]" || strings.TrimSpace(trimmedForZero) == "::" {
				// Bare IPv6 unspecified without port is also considered zero/unspecified
				// ValidateBindHost will reject it, but for dev exception we treat as zero-like
				// to keep dev exception narrowly for 0.0.0.0 only, so not needed here.
			}
		}
		// Also handle [::]:port and :: cases via ValidateBindHost's IsUnspecified check;
		// isZero for dev exception is strictly 0.0.0.0 variants, not IPv6.
		if !(isZero && cfg.DevelopmentMode) {
			if err := network.ValidateBindHost(bindAddr); err != nil {
				return "", "", "", err
			}
		}
	}

	if bindAddr == "" {
		pa, discErr := network.DiscoverPrivateAddress()
		if discErr != nil {
			return "", "", "", fmt.Errorf("private network unavailable: %w", discErr)
		}
		bindAddr = pa.Address
		if endpointAddr == "" {
			endpointAddr = pa.Address + ":" + port
		}
	} else {
		if endpointAddr == "" {
			trimmedBind := strings.TrimSpace(bindAddr)
			isZeroBind := trimmedBind == "0.0.0.0"
			if !isZeroBind && strings.Contains(trimmedBind, ":") {
				if h, _, err := net.SplitHostPort(trimmedBind); err == nil && h == "0.0.0.0" {
					isZeroBind = true
				}
			}
			if trimmedBind == "localhost" || trimmedBind == "127.0.0.1" {
				endpointAddr = "127.0.0.1:" + port
			} else if isZeroBind || trimmedBind == "" {
				pa, discErr := network.DiscoverPrivateAddress()
				if discErr == nil {
					endpointAddr = pa.Address + ":" + port
				} else {
					endpointAddr = "localhost:" + port
				}
			} else {
				// Avoid host:port:port when SERVER_ADDR already contains a port
				if strings.Contains(trimmedBind, ":") {
					if _, _, err := net.SplitHostPort(trimmedBind); err == nil {
						endpointAddr = trimmedBind
					} else {
						endpointAddr = trimmedBind + ":" + port
					}
				} else {
					endpointAddr = trimmedBind + ":" + port
				}
			}
		} else {
			// F-02: Explicit ENDPOINT_ADDR must be validated before any silent replacement
			if err := invitation.ValidateEndpoint(endpointAddr); err != nil {
				return "", "", "", fmt.Errorf("invalid ENDPOINT_ADDR %q: %w", endpointAddr, err)
			}
		}
		// Handle case where ENDPOINT_ADDR is 0.0.0.0? Keep as is; server already handles
		// but launcher should not expose 0.0.0.0 as endpoint — only for auto-derived.
		if cfg.EndpointAddr == "" && endpointAddr == "0.0.0.0:"+port {
			pa, discErr := network.DiscoverPrivateAddress()
			if discErr == nil {
				endpointAddr = pa.Address + ":" + port
			} else {
				endpointAddr = "localhost:" + port
			}
		}
	}

	// F-02: Validate endpoint before advertisement/bundling (including auto-derived)
	if endpointAddr != "" {
		if err := invitation.ValidateEndpoint(endpointAddr); err != nil {
			return "", "", "", fmt.Errorf("invalid endpoint %q: %w", endpointAddr, err)
		}
	}

	// Handle localhost alias normalization for bind
	// Keep bindAddr as "localhost" if originally localhost; fullBindAddr will be "localhost:port"
	// Server.New will handle listening on localhost.
	// Avoid host:port:port when SERVER_ADDR already contains a port
	if strings.Contains(bindAddr, ":") {
		if _, _, err := net.SplitHostPort(bindAddr); err == nil {
			fullBindAddr = bindAddr
		} else {
			// For bare IPv6 without port (e.g., ::1) SplitHostPort fails; still append
			fullBindAddr = bindAddr + ":" + port
		}
	} else {
		fullBindAddr = bindAddr + ":" + port
	}
	return bindAddr, endpointAddr, fullBindAddr, nil
}

// Server returns the embedded server (before or after Start).
func (l *Launcher) Server() *server.Server { return l.server }

// Endpoint returns the server endpoint that clients should dial.
func (l *Launcher) Endpoint() string { return l.endpoint }

// BindAddr returns the resolved bind address host part.
func (l *Launcher) BindAddr() string { return l.bindAddr }

// Config returns the loaded config.
func (l *Launcher) Config() *config.Config { return l.cfg }

// Start launches the server in background and waits for readiness.
func (l *Launcher) Start() {
	go l.server.Start()
	go l.server.CleanupExpiredInvitations()
	<-l.server.WaitReady()
}

// Shutdown gracefully shuts down the embedded server. Idempotent via server.Shutdown.
func (l *Launcher) Shutdown() {
	if l.server != nil {
		l.server.Shutdown()
	}
}

// IsTraySupported reports whether system tray is available.
// Stub for Step 6: no tray implementation is bundled to avoid heavy dependencies.
func IsTraySupported() bool { return false }
