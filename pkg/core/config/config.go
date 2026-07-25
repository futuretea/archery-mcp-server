package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// StaticConfig contains configuration that is fixed when the server starts.
type StaticConfig struct {
	Port            int           `mapstructure:"port"`
	Listen          string        `mapstructure:"listen"`
	SSEBaseURL      string        `mapstructure:"sse_base_url"`
	LogLevel        string        `mapstructure:"log_level"`
	EnabledTools    []string      `mapstructure:"enabled_tools"`
	DisabledTools   []string      `mapstructure:"disabled_tools"`
	EnabledDomains  []string      `mapstructure:"enabled_domains"`
	DisabledDomains []string      `mapstructure:"disabled_domains"`
	Archery         ArcheryConfig `mapstructure:"archery"`
}

type ArcheryConfig struct {
	BaseURL        string        `mapstructure:"base_url"`
	Username       string        `mapstructure:"username"`
	PasswordEnv    string        `mapstructure:"password_env"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
}

// Validate checks whether the configuration can be used to start the server.
func (c *StaticConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", c.Port)
	}
	if c.Port != 0 && strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen must be set when port is non-zero")
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level %q: %w", c.LogLevel, err)
	}
	if strings.TrimSpace(c.Archery.BaseURL) == "" {
		return fmt.Errorf("archery base_url must be set")
	}
	if parsed, err := url.Parse(c.Archery.BaseURL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("archery base_url must be an absolute URL")
	}
	if strings.TrimSpace(c.Archery.Username) == "" {
		return fmt.Errorf("archery username must be set")
	}
	if strings.TrimSpace(c.Archery.PasswordEnv) == "" {
		return fmt.Errorf("archery password_env must be set")
	}
	if strings.TrimSpace(os.Getenv(c.Archery.PasswordEnv)) == "" {
		return fmt.Errorf("archery password environment %q is not set", c.Archery.PasswordEnv)
	}
	if c.Archery.RequestTimeout <= 0 {
		return fmt.Errorf("archery request_timeout must be positive")
	}
	return nil
}

// GetListenAddress returns the configured HTTP listen address (host:port).
func (c *StaticConfig) GetListenAddress() string {
	if c.Port == 0 {
		return ""
	}
	return net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
}

// LoadConfig loads configuration from defaults, optional YAML, environment, and flags.
func LoadConfig(configPath string, v *viper.Viper) (*StaticConfig, error) {
	if v == nil {
		v = viper.New()
	}

	defaults := map[string]any{
		"port":                    0,
		"listen":                  "127.0.0.1",
		"sse_base_url":            "",
		"log_level":               "info",
		"enabled_tools":           []string{},
		"disabled_tools":          []string{},
		"enabled_domains":         []string{},
		"disabled_domains":        []string{},
		"archery.request_timeout": "15s",
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cfg := &StaticConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.EnabledTools = normalizeStringSlice(cfg.EnabledTools)
	cfg.DisabledTools = normalizeStringSlice(cfg.DisabledTools)
	cfg.EnabledDomains = normalizeStringSlice(cfg.EnabledDomains)
	cfg.DisabledDomains = normalizeStringSlice(cfg.DisabledDomains)
	cfg.Archery.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.Archery.BaseURL), "/")
	cfg.Archery.Username = strings.TrimSpace(cfg.Archery.Username)
	cfg.Archery.PasswordEnv = strings.TrimSpace(cfg.Archery.PasswordEnv)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
