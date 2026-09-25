package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	DefaultPort       = 9090
	DefaultRoomTTL    = 600 // seconds
	DefaultRoomTTLDur = 10 * time.Minute
)

// fileConfig mirrors config.json fields. Pointers detect presence.
type fileConfig struct {
	Port         *int    `json:"port,omitempty"`
	ServerAddr   *string `json:"server_addr,omitempty"`
	EndpointAddr *string `json:"endpoint_addr,omitempty"`
	RoomTTL      *int    `json:"room_ttl,omitempty"`
}

// Config holds the effective resolved configuration.
type Config struct {
	Port            int
	ServerAddr      string
	EndpointAddr    string
	RoomTTL         time.Duration
	DevelopmentMode bool
	CertDir         string
	ConfigDir       string
	ConfigPath      string
}

// GetAppDataDir returns the base APPDATA directory.
// Precedence: $APPDATA env var > os.UserConfigDir() > os.UserHomeDir() > ".".
func GetAppDataDir() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

// DefaultConfigDir returns %APPDATA%\TerminalRoom
func DefaultConfigDir() string {
	return filepath.Join(GetAppDataDir(), "TerminalRoom")
}

// DefaultCertDir returns %APPDATA%\TerminalRoom\certs
func DefaultCertDir() string {
	return filepath.Join(DefaultConfigDir(), "certs")
}

// DefaultConfigPath returns %APPDATA%\TerminalRoom\config.json
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.json")
}

// CertDir resolves the certificate directory with precedence:
// CERT_DIR env var > %APPDATA%\TerminalRoom\certs
func CertDir() string {
	if v := os.Getenv("CERT_DIR"); v != "" {
		return v
	}
	return DefaultCertDir()
}

// Load loads configuration from DefaultConfigPath with precedence:
// env var > config.json > defaults.
// Missing config.json is ignored (defaults), but explicitly invalid values return errors.
func Load() (*Config, error) {
	return LoadFromPath(DefaultConfigPath())
}

// LoadFromPath loads from an explicit path (used in tests).
func LoadFromPath(path string) (*Config, error) {
	cfg := &Config{
		Port:            DefaultPort,
		RoomTTL:         DefaultRoomTTLDur,
		ConfigPath:      path,
		ConfigDir:       filepath.Dir(path),
		CertDir:         CertDir(),
		DevelopmentMode: os.Getenv("DEVELOPMENT_MODE") == "1",
	}

	// If path is the default path but we are testing with APPDATA override,
	// ensure ConfigDir/CertDir reflect current APPDATA resolution.
	// For explicit test paths, ConfigDir is dir of path; CertDir still via CertDir().
	if path == DefaultConfigPath() {
		cfg.ConfigDir = DefaultConfigDir()
		// CertDir already resolved via CertDir() which uses DefaultCertDir()
	} else {
		// For non-default path (tests), keep CertDir as resolved via env or default.
		// ConfigDir already set to dir of path.
	}

	// Load file if present
	if data, err := os.ReadFile(path); err == nil {
		var fc fileConfig
		if err := json.Unmarshal(data, &fc); err != nil {
			return nil, fmt.Errorf("invalid config.json: %w", err)
		}
		if fc.Port != nil {
			if *fc.Port < 1 || *fc.Port > 65535 {
				return nil, fmt.Errorf("invalid port in config.json %q: must be integer 1-65535", fmt.Sprintf("%d", *fc.Port))
			}
			cfg.Port = *fc.Port
		}
		if fc.ServerAddr != nil {
			cfg.ServerAddr = *fc.ServerAddr
		}
		if fc.EndpointAddr != nil {
			cfg.EndpointAddr = *fc.EndpointAddr
		}
		if fc.RoomTTL != nil {
			if *fc.RoomTTL <= 0 {
				return nil, fmt.Errorf("invalid room_ttl in config.json %q: must be positive integer", fmt.Sprintf("%d", *fc.RoomTTL))
			}
			cfg.RoomTTL = time.Duration(*fc.RoomTTL) * time.Second
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config %q: %w", path, err)
	}
	// else missing file → use defaults (not an error)

	// Environment overrides (highest precedence) — explicitly invalid must error, not fall through
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid PORT %q: must be integer 1-65535", v)
		}
		cfg.Port = p
	}
	if v := os.Getenv("SERVER_ADDR"); v != "" {
		cfg.ServerAddr = v
	}
	if v := os.Getenv("ENDPOINT_ADDR"); v != "" {
		cfg.EndpointAddr = v
	}
	if v := os.Getenv("ROOM_TTL"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			return nil, fmt.Errorf("invalid ROOM_TTL %q: must be positive integer", v)
		}
		cfg.RoomTTL = time.Duration(secs) * time.Second
	}
	// DEVELOPMENT_MODE already set above; re-evaluate in case env changed after initial
	cfg.DevelopmentMode = os.Getenv("DEVELOPMENT_MODE") == "1"

	// CertDir final resolution (env > default) — re-resolve after file+env
	cfg.CertDir = CertDir()

	// Ensure ConfigDir for default path reflects current APPDATA
	if path == DefaultConfigPath() {
		cfg.ConfigDir = DefaultConfigDir()
	}

	return cfg, nil
}

// PortString returns port as string for net.JoinHostPort / main.go compatibility.
func (c *Config) PortString() string {
	return strconv.Itoa(c.Port)
}

// RoomTTLSeconds returns TTL in seconds.
func (c *Config) RoomTTLSeconds() int {
	return int(c.RoomTTL.Seconds())
}

// EnsureDirs creates ConfigDir and CertDir with 0700 permissions if needed.
func (c *Config) EnsureDirs() error {
	if err := os.MkdirAll(c.ConfigDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(c.CertDir, 0700); err != nil {
		return err
	}
	return nil
}

// EnsureDefaultDirs creates the default config and cert directories.
// Convenience for main.go startup.
func EnsureDefaultDirs() error {
	cfg := &Config{
		ConfigDir: DefaultConfigDir(),
		CertDir:   CertDir(),
	}
	return cfg.EnsureDirs()
}
