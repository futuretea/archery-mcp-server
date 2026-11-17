package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuretea/archery-mcp-server/internal/cmd"
)

func TestToolsListNameListBoundary(t *testing.T) {
	const canary = "must-not-appear-in-errors"
	t.Setenv("ARCHERY_TEST_PASSWORD", canary)
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.yaml")
	configBody := "unrelated: " + canary + "\narchery:\n  base_url: https://archery.example.test/prefix\n  username: mcp-service\n  password_env: ARCHERY_TEST_PASSWORD\n  request_timeout: 15s\nenabled_tools:\n  - users.list\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("empty env preserves yaml", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "")
		names, err := listToolNames(t, "--config", configPath)
		if err != nil {
			t.Fatalf("tools list: %v", err)
		}
		if strings.Join(names, ",") != "users.list" {
			t.Fatalf("tools = %v, want users.list", names)
		}
	})

	t.Run("blank env fails without listing tools", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " , ")
		out, err := runToolsList(t, "--config", configPath)
		if err == nil || !strings.Contains(err.Error(), "enabled_tools contains no names") {
			t.Fatalf("tools list error = %v", err)
		}
		if strings.Contains(out, "users.list") || strings.Contains(out, "workflows.submit") || strings.Contains(out, canary) {
			t.Fatalf("tools list output = %q", out)
		}
	})

	t.Run("changed flag lists trimmed names", func(t *testing.T) {
		names, err := listToolNames(t, "--config", configPath, "--enabled-tools", " users.list , , groups.list ")
		if err != nil {
			t.Fatalf("tools list: %v", err)
		}
		if strings.Join(names, ",") != "groups.list,users.list" && strings.Join(names, ",") != "users.list,groups.list" {
			t.Fatalf("tools = %v, want users.list and groups.list", names)
		}
		for _, name := range names {
			if name == "workflows.submit" {
				t.Fatalf("tools = %v, want only the named tools", names)
			}
		}
	})

	t.Run("changed blank flag fails closed", func(t *testing.T) {
		out, err := runToolsList(t, "--config", configPath, "--enabled-tools", " , ")
		if err == nil || !strings.Contains(err.Error(), "enabled_tools contains no names") {
			t.Fatalf("tools list error = %v, output %q", err, out)
		}
		if strings.Contains(out, "workflows.submit") {
			t.Fatalf("blank flag listed tools: %s", out)
		}
	})

	t.Run("unknown name does not fail the list", func(t *testing.T) {
		names, err := listToolNames(t, "--config", configPath, "--enabled-tools", "missing")
		if err != nil {
			t.Fatalf("tools list: %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("tools = %v, want none", names)
		}
	})

	t.Run("blank disabled tools fail closed", func(t *testing.T) {
		_, err := listToolNames(t, "--config", configPath, "--disabled-tools", " ")
		if err == nil || !strings.Contains(err.Error(), "disabled_tools contains no names") {
			t.Fatalf("tools list error = %v", err)
		}
	})

	t.Run("blank domain flag fails closed", func(t *testing.T) {
		_, err := listToolNames(t, "--config", configPath, "--enable-domains", " ")
		if err == nil || !strings.Contains(err.Error(), "enabled_domains contains no names") {
			t.Fatalf("tools list error = %v", err)
		}
	})

	t.Run("named domain lists that domain", func(t *testing.T) {
		names, err := listToolNames(t, "--config", configPath, "--enable-domains", "archery")
		if err != nil {
			t.Fatalf("tools list: %v", err)
		}
		if strings.Join(names, ",") != "users.list" {
			t.Fatalf("tools = %v, want users.list", names)
		}
	})
}

func listToolNames(t *testing.T, args ...string) ([]string, error) {
	t.Helper()
	out, err := runToolsList(t, args...)
	if err != nil {
		return nil, err
	}
	var listed []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("unmarshal tools: %v\n%s", err, out)
	}
	names := make([]string, 0, len(listed))
	for _, tool := range listed {
		names = append(names, tool.Name)
	}
	return names, nil
}

func runToolsList(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs(append([]string{"tools", "list", "--json"}, args...))
	err := root.Execute()
	return out.String(), err
}
