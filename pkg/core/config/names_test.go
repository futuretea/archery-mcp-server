package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/futuretea/archery-mcp-server/pkg/core/config"
)

func TestLoadConfigNameListBoundary(t *testing.T) {
	const canary = "must-not-appear-in-errors"
	base := "unrelated: " + canary + "\narchery:\n  base_url: https://archery.example.test/prefix\n  username: mcp-service\n  password_env: ARCHERY_TEST_PASSWORD\n  request_timeout: 15s\n"
	allow := base + "enabled_tools:\n  - users.list\ndisabled_tools:\n  - groups.list\n"
	tests := []struct {
		name         string
		yaml         string
		env          map[string]string
		changedFlag  string
		flagValue    string
		parseFlag    bool
		wantErr      string
		wantEnabled  []string
		wantDisabled []string
		wantDomains  []string
		wantOff      []string
	}{
		{
			name:         "omitted lists stay unrestricted",
			yaml:         base,
			wantEnabled:  []string{},
			wantDisabled: []string{},
			wantDomains:  []string{},
			wantOff:      []string{},
		},
		{
			name:         "empty yaml lists stay unrestricted",
			yaml:         base + "enabled_tools: []\ndisabled_tools: []\nenabled_domains: []\ndisabled_domains: []\n",
			wantEnabled:  []string{},
			wantDisabled: []string{},
			wantDomains:  []string{},
			wantOff:      []string{},
		},
		{
			name:        "empty scalar stays unrestricted",
			yaml:        base + "enabled_tools: \"\"\n",
			wantEnabled: []string{},
		},
		{
			name:        "null scalar stays unrestricted",
			yaml:        base + "enabled_tools: null\n",
			wantEnabled: []string{},
		},
		{
			name:         "empty env preserves yaml allow list",
			yaml:         allow,
			env:          map[string]string{"MCP_ENABLED_TOOLS": ""},
			wantEnabled:  []string{"users.list"},
			wantDisabled: []string{"groups.list"},
		},
		{
			name:        "comma separated env trims mixed blanks",
			yaml:        base + "enabled_tools:\n  - workflows.submit\n",
			env:         map[string]string{"MCP_ENABLED_TOOLS": " users.list , , groups.list "},
			wantEnabled: []string{"users.list", "groups.list"},
		},
		{
			name:        "yaml mixed blanks keep names",
			yaml:        base + "enabled_tools:\n  - \" users.list \"\n  - \"\"\n  - groups.list\n",
			wantEnabled: []string{"users.list", "groups.list"},
		},
		{
			name:        "unknown tool name remains a name",
			yaml:        base + "enabled_tools:\n  - missing\n",
			wantEnabled: []string{"missing"},
		},
		{
			name:    "space only env fails closed",
			yaml:    allow,
			env:     map[string]string{"MCP_ENABLED_TOOLS": " "},
			wantErr: "enabled_tools contains no names",
		},
		{
			name:    "comma only env fails closed",
			yaml:    allow,
			env:     map[string]string{"MCP_ENABLED_TOOLS": ","},
			wantErr: "enabled_tools contains no names",
		},
		{
			name:    "blank comma env fails closed",
			yaml:    allow,
			env:     map[string]string{"MCP_ENABLED_TOOLS": " , "},
			wantErr: "enabled_tools contains no names",
		},
		{
			name:    "yaml blank items fail closed",
			yaml:    base + "enabled_tools:\n  - \"\"\n  - \"   \"\n",
			wantErr: "enabled_tools contains no names",
		},
		{
			name:    "blank disabled tools fail closed",
			yaml:    base + "disabled_tools:\n  - \" \"\n",
			wantErr: "disabled_tools contains no names",
		},
		{
			name:    "blank enabled domains fail closed",
			yaml:    base + "enabled_domains:\n  - \" \"\n",
			wantErr: "enabled_domains contains no names",
		},
		{
			name:    "blank disabled domains fail closed",
			yaml:    base + "disabled_domains: [\"\", \" \"]\n",
			wantErr: "disabled_domains contains no names",
		},
		{
			name:         "unchanged flag preserves yaml",
			yaml:         allow,
			changedFlag:  "enabled-tools",
			wantEnabled:  []string{"users.list"},
			wantDisabled: []string{"groups.list"},
		},
		{
			name:        "changed flag overrides yaml and env",
			yaml:        allow,
			env:         map[string]string{"MCP_ENABLED_TOOLS": "users.list"},
			changedFlag: "enabled-tools",
			flagValue:   " users.list , , groups.list ",
			parseFlag:   true,
			wantEnabled: []string{"users.list", "groups.list"},
		},
		{
			name:        "changed blank flag fails closed",
			yaml:        allow,
			changedFlag: "enabled-tools",
			flagValue:   " , ",
			parseFlag:   true,
			wantErr:     "enabled_tools contains no names",
		},
		{
			name:        "changed blank disabled flag fails closed",
			yaml:        base + "disabled_tools:\n  - groups.list\n",
			changedFlag: "disabled-tools",
			flagValue:   ",",
			parseFlag:   true,
			wantErr:     "disabled_tools contains no names",
		},
		{
			name:        "mixed domain env keeps names",
			yaml:        base,
			env:         map[string]string{"MCP_ENABLED_DOMAINS": " archery , , "},
			wantDomains: []string{"archery"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARCHERY_TEST_PASSWORD", canary)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			v := viper.New()
			if tt.changedFlag != "" {
				bindNameFlags(t, v, tt.parseFlag, tt.changedFlag, tt.flagValue)
			}
			cfg, err := config.LoadConfig(path, v)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadConfig error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadConfig error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				if strings.Contains(err.Error(), canary) {
					t.Fatalf("LoadConfig error leaked config value: %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if tt.wantEnabled != nil && !reflect.DeepEqual(cfg.EnabledTools, tt.wantEnabled) {
				t.Fatalf("EnabledTools = %#v, want %#v", cfg.EnabledTools, tt.wantEnabled)
			}
			if tt.wantDisabled != nil && !reflect.DeepEqual(cfg.DisabledTools, tt.wantDisabled) {
				t.Fatalf("DisabledTools = %#v, want %#v", cfg.DisabledTools, tt.wantDisabled)
			}
			if tt.wantDomains != nil && !reflect.DeepEqual(cfg.EnabledDomains, tt.wantDomains) {
				t.Fatalf("EnabledDomains = %#v, want %#v", cfg.EnabledDomains, tt.wantDomains)
			}
			if tt.wantOff != nil && !reflect.DeepEqual(cfg.DisabledDomains, tt.wantOff) {
				t.Fatalf("DisabledDomains = %#v, want %#v", cfg.DisabledDomains, tt.wantOff)
			}
		})
	}
}

func bindNameFlags(t *testing.T, v *viper.Viper, parse bool, name, value string) {
	t.Helper()
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	flags.StringSlice("enabled-tools", []string{}, "")
	flags.StringSlice("disabled-tools", []string{}, "")
	flags.StringSlice("enable-domains", []string{}, "")
	flags.StringSlice("disable-domains", []string{}, "")
	if parse {
		if err := flags.Parse([]string{"--" + name, value}); err != nil {
			t.Fatalf("parse flag: %v", err)
		}
	}
	bindings := map[string]string{
		"enabled_tools":    "enabled-tools",
		"disabled_tools":   "disabled-tools",
		"enabled_domains":  "enable-domains",
		"disabled_domains": "disable-domains",
	}
	for key, flag := range bindings {
		if err := v.BindPFlag(key, flags.Lookup(flag)); err != nil {
			t.Fatalf("bind flag %s: %v", flag, err)
		}
	}
}
