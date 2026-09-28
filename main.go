package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/terminalroom/terminalroom/internal/client"
	"github.com/terminalroom/terminalroom/internal/config"
	"github.com/terminalroom/terminalroom/internal/invitation"
	"github.com/terminalroom/terminalroom/internal/launcher"
	"github.com/terminalroom/terminalroom/internal/network"
	"github.com/terminalroom/terminalroom/internal/server"
	"github.com/terminalroom/terminalroom/internal/terminal"
)

func main() {
	// Enable Windows Virtual Terminal processing for ANSI rendering (e.g., \033[K).
	// Gracefully no-ops on non-Windows or redirected output; never fails startup.
	_ = terminal.EnableVirtualTerminal()
	defer terminal.RestoreVirtualTerminal()

	if len(os.Args) < 2 {
		runLauncher()
		return
	}

	switch os.Args[1] {
	case "server":
		runServer()
	case "client":
		runClient()
	case "diagnose":
		runDiagnose()
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runLauncher() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	_ = cfg.EnsureDirs()

	// Use launcher.New for consistent address resolution and validation.
	l, err := launcher.New(cfg)
	if err != nil {
		// Provide clear user-facing errors matching server mode style.
		msg := err.Error()
		if isPrivateNetworkUnavailable(msg) {
			terminal.Println("")
			terminal.Println(terminal.Banner())
			terminal.Println("              TERMINALROOM")
			terminal.Println(terminal.Banner())
			terminal.Println("")
			terminal.Println("Private network unavailable.")
			terminal.Println("Cannot create a private TerminalRoom.")
			terminal.Println("")
			terminal.Println("To run in development mode, set:")
			terminal.Println("  SERVER_ADDR=localhost")
			terminal.Println("")
			terminal.Println("To use a specific interface, set:")
			terminal.Println("  SERVER_ADDR=<ip-address>")
			terminal.Println("  ENDPOINT_ADDR=<address:port>")
			terminal.Println("")
			diag := network.Diagnostic{
				Available: false,
				Error:     err.Error(),
			}
			terminal.Println(diag.String())
			os.Exit(1)
		}
		if isZeroBindingError(msg) {
			fmt.Fprintln(os.Stderr, "ERROR: Binding to 0.0.0.0 (all interfaces) is not allowed in normal mode.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "This would expose the server on public networks.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To bind to a private interface, set SERVER_ADDR to a private IP.")
			fmt.Fprintln(os.Stderr, "To bind to localhost for development, set SERVER_ADDR=localhost")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To explicitly allow 0.0.0.0 binding (DEVELOPMENT ONLY):")
			fmt.Fprintln(os.Stderr, "  SERVER_ADDR=0.0.0.0 DEVELOPMENT_MODE=1 terminalroom")
			os.Exit(1)
		}
		if isPortInUseError(msg) {
			terminal.Println("")
			terminal.Println(terminal.Banner())
			terminal.Println("              TERMINALROOM")
			terminal.Println(terminal.Banner())
			terminal.Println("")
			terminal.Println("Port already in use.")
			terminal.Printf("Cannot start TerminalRoom on %s\n", cfg.PortString())
			terminal.Println("")
			terminal.Println("Another TerminalRoom or service may be using this port.")
			terminal.Println("Close the other instance or set a different port:")
			terminal.Println("  PORT=9091 terminalroom")
			terminal.Println("")
			fmt.Fprintf(os.Stderr, "Detail: %v\n", err)
			os.Exit(1)
		}
		// Generic certificate or other startup failure
		if isCertError(msg) {
			terminal.Println("")
			terminal.Println("Certificate initialization failed.")
			terminal.Printf("Cert dir: %s\n", cfg.CertDir)
			terminal.Println("")
			fmt.Fprintf(os.Stderr, "Detail: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Failed to start TerminalRoom: %v\n", err)
		os.Exit(1)
	}

	// Handle window close / Ctrl+C even before client starts.
	// Launcher Shutdown is idempotent (sync.Once) so this handler safely
	// coexists with the client's internal signal handling; both call the same
	// l.Shutdown() -> Server.Shutdown() without a second independent mechanism.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		l.Shutdown()
		os.Exit(0)
	}()
	defer signal.Stop(sigCh)

	if l.BindAddr() == "0.0.0.0" && l.Config().DevelopmentMode {
		fmt.Fprintln(os.Stderr, "WARNING: Binding to 0.0.0.0 (all interfaces). DEVELOPMENT MODE.")
		fmt.Fprintln(os.Stderr, "This server is accessible on public networks. Do not use in production.")
	}

	// Start embedded server in background with deterministic readiness.
	// No arbitrary sleep; WaitReady is deterministic via server's ready channel.
	l.Start()
	defer l.Shutdown()

	// Endpoint is the actual private/server endpoint, not blindly localhost:9090.
	endpoint := l.Endpoint()
	certDir := cfg.CertDir

	// Start client; client owns interactive terminal.
	c := client.New(endpoint, certDir)
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// Ensure server is shutdown before exit.
		l.Shutdown()
		os.Exit(1)
	}
	// Client exited normally -> shutdown server and exit cleanly.
	l.Shutdown()
}

func isPrivateNetworkUnavailable(msg string) bool {
	return contains(msg, "private network unavailable") || contains(msg, "no usable private network") || contains(msg, "no private network")
}

func isZeroBindingError(msg string) bool {
	return contains(msg, "0.0.0.0")
}

func isPortInUseError(msg string) bool {
	return contains(msg, "already in use") || contains(msg, "port already in use")
}

func isCertError(msg string) bool {
	return contains(msg, "certificate") || contains(msg, "cert") || contains(msg, "TLS") || contains(msg, "tls")
}

func contains(s, substr string) bool {
	// case-insensitive
	return len(s) >= len(substr) && (func() bool {
		ls := toLower(s)
		lsub := toLower(substr)
		return stringContains(ls, lsub)
	})()
}

func toLower(s string) string {
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

func stringContains(s, substr string) bool {
	if substr == "" {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func runServer() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	_ = cfg.EnsureDirs()

	port := cfg.PortString()
	certDir := cfg.CertDir
	roomTTL := cfg.RoomTTL
	bindAddr := strings.TrimSpace(cfg.ServerAddr)
	endpointAddr := strings.TrimSpace(cfg.EndpointAddr)

	// F-01: Reject explicit public SERVER_ADDR (private/loopback/tailscale only, or empty for auto)
	if bindAddr != "" {
		trimmedZero := strings.TrimSpace(bindAddr)
		isZero := trimmedZero == "0.0.0.0"
		if !isZero && strings.Contains(trimmedZero, ":") {
			if h, _, err := net.SplitHostPort(trimmedZero); err == nil && h == "0.0.0.0" {
				isZero = true
			}
		}
		if !(isZero && cfg.DevelopmentMode) {
			if err := network.ValidateBindHost(bindAddr); err != nil {
				// Preserve existing 0.0.0.0 error style but surface public error clearly
				fmt.Fprintln(os.Stderr, err.Error())
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "TerminalRoom requires a private network address.")
				fmt.Fprintln(os.Stderr, "Use a private address (10.x, 172.16-31.x, 192.168.x, 100.64-127.x) or localhost.")
				os.Exit(1)
			}
		}
	}

	if bindAddr == "" {
		pa, err := network.DiscoverPrivateAddress()
		if err != nil {
			terminal.Println("")
			terminal.Println(terminal.Banner())
			terminal.Println("              TERMINALROOM SERVER")
			terminal.Println(terminal.Banner())
			terminal.Println("")
			terminal.Println("Private network unavailable.")
			terminal.Println("Cannot create a private TerminalRoom.")
			terminal.Println("")
			terminal.Println("To run in development mode, set:")
			terminal.Println("  SERVER_ADDR=localhost")
			terminal.Println("")
			terminal.Println("To use a specific interface, set:")
			terminal.Println("  SERVER_ADDR=<ip-address>")
			terminal.Println("  ENDPOINT_ADDR=<address:port>")
			terminal.Println("")
			diag := network.Diagnostic{
				Available: false,
				Error:     err.Error(),
			}
			terminal.Println(diag.String())
			os.Exit(1)
		}

		bindAddr = pa.Address
		if endpointAddr == "" {
			endpointAddr = pa.Address + ":" + port
		}

		terminal.Println("")
		terminal.Println(terminal.Banner())
		terminal.Println("              TERMINALROOM SERVER")
		terminal.Println(terminal.Banner())
		terminal.Println("")
		terminal.Printf("Private network detected : %s\n", "YES")
		terminal.Printf("Private address          : %s\n", pa.Address)
		terminal.Printf("Network type             : %s\n", pa.NetworkType.String())
		terminal.Println("")
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
				pa, err := network.DiscoverPrivateAddress()
				if err == nil {
					endpointAddr = pa.Address + ":" + port
				} else {
					endpointAddr = "localhost:" + port
				}
			} else {
				// Avoid host:port:port when SERVER_ADDR already contains port
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
		}

		terminal.Println("")
		terminal.Println(terminal.Banner())
		terminal.Println("              TERMINALROOM SERVER")
		terminal.Println(terminal.Banner())
		terminal.Println("")
		if bindAddr == "localhost" || bindAddr == "127.0.0.1" {
			terminal.Println("Private network detected : NO (development mode)")
			terminal.Printf("Bind address             : %s\n", bindAddr)
			terminal.Println("Network type             : local")
		} else {
			terminal.Printf("Bind address             : %s\n", bindAddr)
			terminal.Println("Network type             : private")
		}
		terminal.Println("")
	}

	// Avoid host:port:port when SERVER_ADDR already contains port
	var fullBindAddr string
	if strings.Contains(bindAddr, ":") {
		if _, _, err := net.SplitHostPort(bindAddr); err == nil {
			fullBindAddr = bindAddr
		} else {
			fullBindAddr = bindAddr + ":" + port
		}
	} else {
		fullBindAddr = bindAddr + ":" + port
	}

	// F-02: Validate explicit ENDPOINT_ADDR (private/loopback/tailscale only, host:port required)
	if cfg.EndpointAddr != "" {
		if err := invitation.ValidateEndpoint(endpointAddr); err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf("invalid ENDPOINT_ADDR %q: %v", endpointAddr, err))
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "TerminalRoom requires a private endpoint (host:port with 10.x, 172.16-31.x, 192.168.x, 100.64-127.x, localhost or 127.0.0.1).")
			os.Exit(1)
		}
	}

	if bindAddr == "0.0.0.0" {
		if !cfg.DevelopmentMode {
			fmt.Fprintln(os.Stderr, "ERROR: Binding to 0.0.0.0 (all interfaces) is not allowed in normal mode.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "This would expose the server on public networks.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To bind to a private interface, set SERVER_ADDR to a private IP.")
			fmt.Fprintln(os.Stderr, "To bind to localhost for development, set SERVER_ADDR=localhost")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To explicitly allow 0.0.0.0 binding (DEVELOPMENT ONLY):")
			fmt.Fprintln(os.Stderr, "  SERVER_ADDR=0.0.0.0 DEVELOPMENT_MODE=1 terminalroom server")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "WARNING: Binding to 0.0.0.0 (all interfaces). DEVELOPMENT MODE.")
		fmt.Fprintln(os.Stderr, "This server is accessible on public networks. Do not use in production.")
	}

	srv, err := server.NewWithTTLAndEndpoint(fullBindAddr, certDir, roomTTL, endpointAddr)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down server...")
		srv.Shutdown()
	}()

	go srv.CleanupExpiredInvitations()

	terminal.Printf("Listening                : %s\n", srv.Addr())
	terminal.Println("Transport                : TLS")
	terminal.Printf("Endpoint                 : %s\n", srv.Endpoint())
	terminal.Println("")
	terminal.Println("Press Ctrl+C to shut down.")
	terminal.Println("")

	srv.Start()
}

func runClient() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	_ = cfg.EnsureDirs()

	addr := "localhost:9090"
	if cfg.ServerAddr != "" {
		addr = cfg.ServerAddr
		// Normalize host-only addresses like "localhost" to include port using config PORT.
		if !contains(addr, ":") {
			addr = addr + ":" + cfg.PortString()
			// Preserve localhost alias as 127.0.0.1 for consistency
			if addr == "localhost:"+cfg.PortString() {
				addr = "127.0.0.1:" + cfg.PortString()
			}
		} else if addr == "localhost:"+cfg.PortString() {
			addr = "127.0.0.1:" + cfg.PortString()
		}
	}

	certDir := cfg.CertDir

	c := client.New(addr, certDir)
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDiagnose() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	terminal.Println("")
	terminal.Println(terminal.Banner())
	terminal.Println("          TERMINALROOM NETWORK DIAGNOSTICS")
	terminal.Println(terminal.Banner())
	terminal.Println("")

	pa, err := network.DiscoverPrivateAddress()
	if err != nil {
		diag := network.Diagnostic{
			Available: false,
			Error:     err.Error(),
		}
		terminal.Println(diag.String())
		terminal.Println("")
		terminal.Println("To run TerminalRoom without a private network:")
		terminal.Println("  SERVER_ADDR=localhost terminalroom server")
		return
	}

	terminal.Printf("Private network detected : %s\n", "YES")
	terminal.Printf("Private address          : %s\n", pa.Address)
	terminal.Printf("Private interface        : %s\n", pa.InterfaceName)
	terminal.Printf("Network type             : %s\n", pa.NetworkType.String())
	terminal.Println("")
	terminal.Printf("To start server on this interface:\n")
	terminal.Printf("  SERVER_ADDR=%s terminalroom server\n", pa.Address)
	terminal.Println("")
	terminal.Printf("Clients connect to: %s\n", pa.Address+":"+cfg.PortString())
}

func printUsage() {
	terminal.Println("")
	terminal.Println(terminal.Banner())
	terminal.Println("              TERMINALROOM")
	terminal.Println("       PRIVATE • TEMPORARY • TWO-PERSON")
	terminal.Println(terminal.Banner())
	terminal.Println("")
	terminal.Println("Usage:")
	terminal.Println("  terminalroom               Start TerminalRoom (server + client)")
	terminal.Println("  terminalroom server        Start the server only")
	terminal.Println("  terminalroom client        Start the client only")
	terminal.Println("  terminalroom diagnose      Show private network diagnostics")
	terminal.Println("  terminalroom help          Show this help")
	terminal.Println("")
	terminal.Println("Configuration:")
	terminal.Printf("  Config file      %s\n", config.DefaultConfigPath())
	terminal.Printf("  Cert directory   %s\n", config.DefaultCertDir())
	terminal.Println("  (No manual setup required for normal use)")
	terminal.Println("")
	terminal.Println("Environment variables (override config file):")
	terminal.Println("  PORT             Server port (default: 9090)")
	terminal.Println("  SERVER_ADDR      Server bind address (default: auto-detect private)")
	terminal.Println("  ENDPOINT_ADDR    Advertised endpoint address (default: auto-detect)")
	terminal.Printf("  CERT_DIR         Certificate directory (default: %s)\n", config.DefaultCertDir())
	terminal.Println("  ROOM_TTL         Room time-to-live in seconds (default: 600)")
	terminal.Println("  DEVELOPMENT_MODE Set to 1 to allow SERVER_ADDR=0.0.0.0 binding")
	terminal.Println("")
	terminal.Println("Examples:")
	terminal.Println("  # Start TerminalRoom (auto private server + client menu)")
	terminal.Println("  terminalroom")
	terminal.Println("")
	terminal.Println("  # Start server on private network (auto-detect)")
	terminal.Println("  terminalroom server")
	terminal.Println("")
	terminal.Println("  # Start server on localhost (development)")
	terminal.Println("  SERVER_ADDR=localhost terminalroom server")
	terminal.Println("")
	terminal.Println("  # Start server on specific interface")
	terminal.Println("  SERVER_ADDR=192.168.1.100 PORT=8080 terminalroom server")
	terminal.Println("")
	terminal.Println("  # Start client (connect to default)")
	terminal.Println("  terminalroom client")
	terminal.Println("")
	terminal.Println("  # Start client (connect to specific server)")
	terminal.Println("  SERVER_ADDR=192.168.1.100:9090 terminalroom client")
	terminal.Println("")
	terminal.Println("  # Show network diagnostics")
	terminal.Println("  terminalroom diagnose")
	terminal.Println("")
}
