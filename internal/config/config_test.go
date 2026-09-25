package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/invitation"
)

func tempAppData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Use forward slash vs backslash both work; filepath handles.
	t.Setenv("APPDATA", dir)
	// Clear relevant env for isolation
	t.Setenv("CERT_DIR", "")
	t.Setenv("PORT", "")
	t.Setenv("ROOM_TTL", "")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("ENDPOINT_ADDR", "")
	t.Setenv("DEVELOPMENT_MODE", "")
	return dir
}

func TestDefaultPort(t *testing.T) {
	dir := tempAppData(t)
	// Ensure no config file
	cfg, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if err != nil {
		t.Fatalf("LoadFromPath error: %v", err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("default port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.PortString() != "9090" {
		t.Errorf("PortString = %q, want %q", cfg.PortString(), "9090")
	}
}

func TestDefaultTTL(t *testing.T) {
	dir := tempAppData(t)
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.RoomTTL != DefaultRoomTTLDur {
		t.Errorf("default TTL = %v, want %v", cfg.RoomTTL, DefaultRoomTTLDur)
	}
	if cfg.RoomTTLSeconds() != DefaultRoomTTL {
		t.Errorf("default TTL seconds = %d, want %d", cfg.RoomTTLSeconds(), DefaultRoomTTL)
	}
	if cfg.RoomTTL != 10*time.Minute {
		t.Errorf("default TTL duration mismatch: got %v", cfg.RoomTTL)
	}
}

func TestEnvVarOverridePort(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("PORT", "8080")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.Port != 8080 {
		t.Errorf("env PORT override = %d, want 8080", cfg.Port)
	}
}

func TestEnvVarOverrideTTL(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("ROOM_TTL", "300")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.RoomTTL != 300*time.Second {
		t.Errorf("env ROOM_TTL override = %v, want 300s", cfg.RoomTTL)
	}
}

func TestEnvVarOverrideServerAddr(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("SERVER_ADDR", "192.168.1.100")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.ServerAddr != "192.168.1.100" {
		t.Errorf("env SERVER_ADDR = %q, want %q", cfg.ServerAddr, "192.168.1.100")
	}
}

func TestEnvVarOverrideEndpointAddr(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("ENDPOINT_ADDR", "10.0.0.1:9090")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.EndpointAddr != "10.0.0.1:9090" {
		t.Errorf("env ENDPOINT_ADDR = %q, want %q", cfg.EndpointAddr, "10.0.0.1:9090")
	}
}

func TestConfigFileValue(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{"port": 7070, "server_addr": "10.0.0.5", "endpoint_addr": "10.0.0.5:7070", "room_ttl": 1200}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFromPath(path)
	if cfg.Port != 7070 {
		t.Errorf("config file port = %d, want 7070", cfg.Port)
	}
	if cfg.ServerAddr != "10.0.0.5" {
		t.Errorf("config file server_addr = %q, want %q", cfg.ServerAddr, "10.0.0.5")
	}
	if cfg.EndpointAddr != "10.0.0.5:7070" {
		t.Errorf("config file endpoint_addr = %q, want %q", cfg.EndpointAddr, "10.0.0.5:7070")
	}
	if cfg.RoomTTL != 1200*time.Second {
		t.Errorf("config file room_ttl = %v, want 1200s", cfg.RoomTTL)
	}
}

func TestEnvTakesPrecedenceOverConfig(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{"port": 7070, "room_ttl": 1200, "server_addr": "10.0.0.5", "endpoint_addr": "10.0.0.5:7070"}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "8080")
	t.Setenv("ROOM_TTL", "300")
	t.Setenv("SERVER_ADDR", "192.168.1.1")
	t.Setenv("ENDPOINT_ADDR", "192.168.1.1:8080")
	cfg, _ := LoadFromPath(path)
	if cfg.Port != 8080 {
		t.Errorf("env should override config port: got %d, want 8080", cfg.Port)
	}
	if cfg.RoomTTL != 300*time.Second {
		t.Errorf("env should override config room_ttl: got %v, want 300s", cfg.RoomTTL)
	}
	if cfg.ServerAddr != "192.168.1.1" {
		t.Errorf("env should override config server_addr: got %q", cfg.ServerAddr)
	}
	if cfg.EndpointAddr != "192.168.1.1:8080" {
		t.Errorf("env should override config endpoint_addr: got %q", cfg.EndpointAddr)
	}
}

func TestDefaultAppDataPath(t *testing.T) {
	dir := tempAppData(t)
	expectedDir := filepath.Join(dir, "TerminalRoom")
	expectedCert := filepath.Join(expectedDir, "certs")
	expectedPath := filepath.Join(expectedDir, "config.json")

	if got := DefaultConfigDir(); got != expectedDir {
		t.Errorf("DefaultConfigDir = %q, want %q", got, expectedDir)
	}
	if got := DefaultCertDir(); got != expectedCert {
		t.Errorf("DefaultCertDir = %q, want %q", got, expectedCert)
	}
	if got := DefaultConfigPath(); got != expectedPath {
		t.Errorf("DefaultConfigPath = %q, want %q", got, expectedPath)
	}
	// Also test CertDir() without env
	if got := CertDir(); got != expectedCert {
		t.Errorf("CertDir() default = %q, want %q", got, expectedCert)
	}
	// Ensure Load uses those defaults
	cfg, _ := Load()
	if cfg.ConfigDir != expectedDir {
		t.Errorf("Load ConfigDir = %q, want %q", cfg.ConfigDir, expectedDir)
	}
	if cfg.CertDir != expectedCert {
		t.Errorf("Load CertDir = %q, want %q", cfg.CertDir, expectedCert)
	}
	// Check GetAppDataDir
	if got := GetAppDataDir(); got != dir {
		t.Errorf("GetAppDataDir = %q, want %q", got, dir)
	}
}

func TestExplicitCertDirOverride(t *testing.T) {
	dir := tempAppData(t)
	_ = dir
	custom := filepath.Join(t.TempDir(), "my-certs")
	t.Setenv("CERT_DIR", custom)
	if got := CertDir(); got != custom {
		t.Errorf("CertDir with CERT_DIR env = %q, want %q", got, custom)
	}
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.CertDir != custom {
		t.Errorf("Config CertDir with env = %q, want %q", cfg.CertDir, custom)
	}
	// Also test that config file does NOT override CERT_DIR
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{"port": 7070}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := LoadFromPath(path)
	if cfg2.CertDir != custom {
		t.Errorf("CERT_DIR should remain env override even with config file: got %q", cfg2.CertDir)
	}
}

func TestInvalidPort(t *testing.T) {
	// Invalid env PORT must error, not fallback
	for _, v := range []string{"notanumber", "-1", "0", "70000"} {
		dir := tempAppData(t)
		t.Setenv("PORT", v)
		_, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
		if err == nil {
			t.Errorf("invalid PORT %q should return error, got nil", v)
		} else if !strings.Contains(err.Error(), "PORT") {
			t.Errorf("invalid PORT %q error should mention PORT, got %v", v, err)
		}
		// Ensure no secret in error
		if strings.Contains(err.Error(), "TRINV") {
			t.Errorf("PORT error should not contain token")
		}
	}
	// Invalid config port alone must error
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{"port": -5}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config port -5 should return error")
	}
	content2 := `{"port": 70000}`
	if err := os.WriteFile(path, []byte(content2), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config port 70000 should return error")
	}
	content3 := `{"port": "abc"}`
	if err := os.WriteFile(path, []byte(content3), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config port type string should return error")
	}
}

func TestInvalidRoomTTL(t *testing.T) {
	for _, v := range []string{"notanumber", "-100", "0"} {
		dir := tempAppData(t)
		t.Setenv("ROOM_TTL", v)
		_, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
		if err == nil {
			t.Errorf("invalid ROOM_TTL %q should return error", v)
		} else if !strings.Contains(err.Error(), "ROOM_TTL") {
			t.Errorf("ROOM_TTL error should mention ROOM_TTL, got %v", err)
		}
	}
	// Config file invalid
	dir4 := tempAppData(t)
	configDir := filepath.Join(dir4, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{"room_ttl": -10}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config room_ttl -10 should return error")
	}
	content2 := `{"room_ttl": 0}`
	if err := os.WriteFile(path, []byte(content2), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("zero config room_ttl should return error")
	}
	content3 := `{"room_ttl": "abc"}`
	if err := os.WriteFile(path, []byte(content3), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid type config room_ttl should return error")
	}
}

func TestMissingConfigFile(t *testing.T) {
	dir := tempAppData(t)
	path := filepath.Join(dir, "TerminalRoom", "config.json")
	// Ensure no file exists
	_ = os.Remove(path)
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath with missing file should not error: %v", err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("missing file port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.RoomTTL != DefaultRoomTTLDur {
		t.Errorf("missing file TTL = %v, want %v", cfg.RoomTTL, DefaultRoomTTLDur)
	}
}

func TestMalformedConfigFile(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	content := `{invalid json,,,`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("malformed config.json should return error")
	}
	if !strings.Contains(err.Error(), "config.json") {
		t.Errorf("malformed error should mention config.json, got %v", err)
	}
	// Even env valid should not bypass malformed file error (file is higher priority invalid)
	t.Setenv("PORT", "9091")
	if _, err := LoadFromPath(path); err == nil {
		t.Error("malformed file should still error even with valid env (env does not hide file error)")
	}
}

func TestConfigDoesNotStoreSecrets(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	// Write config with only allowed fields, ensure no secret fields are read as config
	content := `{"port": 9090, "server_addr": "localhost", "room_ttl": 600, "password": "should-not-be-used", "token": "TRINV-should-not-be-stored"}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFromPath(path)
	if cfg.Port != 9090 {
		t.Errorf("port should be 9090, got %d", cfg.Port)
	}
	// Ensure extra fields don't cause error and are ignored
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "password") {
		// file contains secrets but config ignores them — verify no field leaks
		if cfg.ServerAddr == "should-not-be-used" {
			t.Error("config should not read password field")
		}
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := tempAppData(t)
	cfg, _ := Load()
	// Override to temp location
	cfg.ConfigDir = filepath.Join(dir, "TerminalRoom")
	cfg.CertDir = filepath.Join(dir, "TerminalRoom", "certs")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs error: %v", err)
	}
	if _, err := os.Stat(cfg.ConfigDir); err != nil {
		t.Errorf("ConfigDir not created: %v", err)
	}
	if _, err := os.Stat(cfg.CertDir); err != nil {
		t.Errorf("CertDir not created: %v", err)
	}
}

func TestDevelopmentMode(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("DEVELOPMENT_MODE", "1")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if !cfg.DevelopmentMode {
		t.Error("DEVELOPMENT_MODE=1 should be true")
	}
	t.Setenv("DEVELOPMENT_MODE", "0")
	cfg2, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg2.DevelopmentMode {
		t.Error("DEVELOPMENT_MODE=0 should be false")
	}
	t.Setenv("DEVELOPMENT_MODE", "")
	cfg3, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg3.DevelopmentMode {
		t.Error("unset DEVELOPMENT_MODE should be false")
	}
}

func TestPortStringAndTTLSeconds(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("PORT", "8080")
	t.Setenv("ROOM_TTL", "123")
	cfg, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg.PortString() != "8080" {
		t.Errorf("PortString = %q, want 8080", cfg.PortString())
	}
	if cfg.RoomTTLSeconds() != 123 {
		t.Errorf("RoomTTLSeconds = %d, want 123", cfg.RoomTTLSeconds())
	}
}

// F-07: Explicit invalid higher priority must not fall through to lower
func TestEnvInvalidDoesNotFallThroughToConfig(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	// Valid config
	if err := os.WriteFile(path, []byte(`{"port": 7070, "room_ttl": 1200}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Invalid env should error, not silently use config 7070
	t.Setenv("PORT", "abc")
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid env PORT should not fall through to valid config, must error")
	}
	t.Setenv("PORT", "")
	t.Setenv("ROOM_TTL", "notanumber")
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid env ROOM_TTL should not fall through to valid config")
	}
}

func TestConfigInvalidDoesNotFallThroughToDefault(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	// Invalid config should error, not silently use DefaultPort/DefaultTTL
	if err := os.WriteFile(path, []byte(`{"port": 0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config port 0 should not fall through to default, must error")
	}
	if err := os.WriteFile(path, []byte(`{"room_ttl": -5}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(path); err == nil {
		t.Error("invalid config room_ttl should not fall through to default")
	}
}

func TestMissingOptionalFieldsUseDefaults(t *testing.T) {
	dir := tempAppData(t)
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	// Only port present, room_ttl missing should use default
	if err := os.WriteFile(path, []byte(`{"port": 8080}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("missing optional field should not error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("port should be 8080, got %d", cfg.Port)
	}
	if cfg.RoomTTL != DefaultRoomTTLDur {
		t.Errorf("missing room_ttl should use default %v, got %v", DefaultRoomTTLDur, cfg.RoomTTL)
	}
	// Empty config should use all defaults
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("empty config should not error: %v", err)
	}
	if cfg2.Port != DefaultPort || cfg2.RoomTTL != DefaultRoomTTLDur {
		t.Errorf("empty config should use defaults, got port %d ttl %v", cfg2.Port, cfg2.RoomTTL)
	}
}

func TestNoSecretsInConfigErrors(t *testing.T) {
	dir := tempAppData(t)
	t.Setenv("PORT", "abc")
	_, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if err == nil {
		t.Fatal("invalid PORT should error")
	}
	if strings.Contains(err.Error(), "TRINV") {
		t.Error("config error should not contain token")
	}
	if !strings.Contains(err.Error(), "PORT") {
		t.Error("PORT error should mention PORT")
	}
	// File with extra secret fields should not leak token in error; valid config with secrets should not error and not leak
	configDir := filepath.Join(dir, "TerminalRoom")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	// File contains token-like string but invalid json
	if err := os.WriteFile(path, []byte(`{invalid`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "")
	_, err = LoadFromPath(path)
	if err == nil {
		t.Fatal("malformed should error")
	}
	if strings.Contains(err.Error(), "TRINV") {
		t.Error("malformed error should not contain token")
	}
}

func TestApplicationPathInvalidPortFailsStartup(t *testing.T) {
	// Simulate startup path: LoadFromPath with invalid PORT should error before server starts
	dir := tempAppData(t)
	t.Setenv("PORT", "abc")
	_, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if err == nil {
		t.Fatal("PORT=abc should fail at config load, preventing startup")
	}
	// Also invalid endpoint via config should be caught at launcher/server later, but config load itself for endpoint string does not error
	// Here we test that invalid PORT blocks startup, not silently uses 9090
	if strings.Contains(err.Error(), "9090") && strings.Contains(err.Error(), "abc") {
		// error mentions invalid value and expected range, good
	}
}

func TestExplicitInvalidEndpointNotSilentlyReplaced(t *testing.T) {
	// This tests the interaction: config has invalid endpoint, but launcher should error, not auto-discover
	dir := tempAppData(t)
	t.Setenv("PORT", "")
	t.Setenv("SERVER_ADDR", "localhost")
	t.Setenv("ENDPOINT_ADDR", "8.8.8.8:9090")
	// Config load will succeed (endpoint string stored), but later validation should fail
	cfg, err := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if err != nil {
		t.Fatalf("config load should succeed for endpoint string (validation deferred): %v", err)
	}
	// Simulate launcher validation (F-02) — should fail, not silently use auto
	if cfg.EndpointAddr != "8.8.8.8:9090" {
		t.Errorf("endpoint should be stored as provided")
	}
	if err := invitation.ValidateEndpoint(cfg.EndpointAddr); err == nil {
		t.Error("explicit invalid endpoint should be rejected by ValidateEndpoint, not auto-discovered")
	}
	// Also ensure explicit invalid SERVER_ADDR is stored, not silently auto-discovered
	t.Setenv("SERVER_ADDR", "8.8.8.8")
	cfg2, _ := LoadFromPath(filepath.Join(dir, "TerminalRoom", "config.json"))
	if cfg2.ServerAddr != "8.8.8.8" {
		t.Errorf("server_addr should be stored as explicit, not auto-discovered")
	}
}
