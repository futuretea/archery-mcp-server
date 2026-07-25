package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// Seam: LoadConfig observes the deployment-owned Archery connection inputs.
func TestLoadConfigDefaultsWithArcheryInputs(t *testing.T) {
	t.Setenv("ARCHERY_TEST_PASSWORD", "synthetic-test-value")
	cfg, err := LoadConfig("", configuredViper())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 0 {
		t.Fatalf("expected port 0, got %d", cfg.Port)
	}
	if cfg.Listen != "127.0.0.1" {
		t.Fatalf("expected listen 127.0.0.1, got %q", cfg.Listen)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected log_level info, got %q", cfg.LogLevel)
	}
	if cfg.Archery.BaseURL != "https://archery.example.test/prefix" {
		t.Fatalf("archery base_url = %q", cfg.Archery.BaseURL)
	}
}

// Seam: StaticConfig.Validate rejects incomplete Archery configuration before client construction.
func TestValidate(t *testing.T) {
	t.Setenv("ARCHERY_TEST_PASSWORD", "synthetic-test-value")
	tests := []struct {
		name    string
		cfg     StaticConfig
		wantSub string
	}{
		{
			name:    "bad port",
			cfg:     StaticConfig{Port: 70000, LogLevel: "info", Listen: "127.0.0.1", Archery: validArcheryConfig()},
			wantSub: "port must be between",
		},
		{
			name:    "empty listen",
			cfg:     StaticConfig{Port: 8080, LogLevel: "info", Listen: " ", Archery: validArcheryConfig()},
			wantSub: "listen must be set",
		},
		{
			name:    "bad log level",
			cfg:     StaticConfig{Port: 0, LogLevel: "nope", Listen: "127.0.0.1", Archery: validArcheryConfig()},
			wantSub: "invalid log_level",
		},
		{
			name:    "missing archery base URL",
			cfg:     StaticConfig{Port: 0, LogLevel: "info", Listen: "127.0.0.1", Archery: ArcheryConfig{Username: "mcp-service", PasswordEnv: "ARCHERY_TEST_PASSWORD", RequestTimeout: time.Second}},
			wantSub: "base_url",
		},
		{
			name:    "missing archery username",
			cfg:     StaticConfig{Port: 0, LogLevel: "info", Listen: "127.0.0.1", Archery: ArcheryConfig{BaseURL: "https://archery.example.test", PasswordEnv: "ARCHERY_TEST_PASSWORD", RequestTimeout: time.Second}},
			wantSub: "username",
		},
		{
			name:    "missing password environment name",
			cfg:     StaticConfig{Port: 0, LogLevel: "info", Listen: "127.0.0.1", Archery: ArcheryConfig{BaseURL: "https://archery.example.test", Username: "mcp-service", RequestTimeout: time.Second}},
			wantSub: "password_env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

// Seam: LoadConfig checks the password environment without including its value in failures.
func TestLoadConfigRejectsMissingPasswordEnvironmentAndRedactsValues(t *testing.T) {
	t.Setenv("ARCHERY_TEST_PASSWORD", "")
	_, err := LoadConfig("", configuredViper())
	if err == nil || !strings.Contains(err.Error(), "ARCHERY_TEST_PASSWORD") {
		t.Fatalf("LoadConfig() error = %v, want missing password environment error", err)
	}

	const syntheticPassword = "must-not-appear-in-errors"
	t.Setenv("ARCHERY_TEST_PASSWORD", syntheticPassword)
	v := configuredViper()
	v.Set("port", -1)
	_, err = LoadConfig("", v)
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want invalid configuration")
	}
	if strings.Contains(err.Error(), syntheticPassword) {
		t.Fatalf("LoadConfig() leaked password in error: %v", err)
	}
}

func TestGetListenAddress(t *testing.T) {
	tests := []struct {
		name string
		cfg  StaticConfig
		want string
	}{
		{name: "stdio", cfg: StaticConfig{Port: 0, Listen: "127.0.0.1"}, want: ""},
		{name: "IPv4", cfg: StaticConfig{Port: 8080, Listen: "127.0.0.1"}, want: "127.0.0.1:8080"},
		{name: "hostname", cfg: StaticConfig{Port: 8080, Listen: "localhost"}, want: "localhost:8080"},
		{name: "IPv6", cfg: StaticConfig{Port: 8080, Listen: "::1"}, want: "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetListenAddress(); got != tt.want {
				t.Fatalf("GetListenAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func configuredViper() *viper.Viper {
	v := viper.New()
	v.Set("archery.base_url", "https://archery.example.test/prefix")
	v.Set("archery.username", "mcp-service")
	v.Set("archery.password_env", "ARCHERY_TEST_PASSWORD")
	v.Set("archery.request_timeout", "15s")
	return v
}

func validArcheryConfig() ArcheryConfig {
	return ArcheryConfig{
		BaseURL:        "https://archery.example.test/prefix",
		Username:       "mcp-service",
		PasswordEnv:    "ARCHERY_TEST_PASSWORD",
		RequestTimeout: 15 * time.Second,
	}
}
