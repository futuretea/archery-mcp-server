package cmd_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/futuretea/archery-mcp-server/internal/cmd"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := out.String()
	for _, want := range []string{"mcp", "tools", "version", "completion"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
}

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("root with no args: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Available Commands") {
		t.Fatalf("expected help output, got:\n%s", text)
	}
	if strings.Contains(text, "registered MCP tools") || strings.Contains(text, "starting HTTP MCP server") {
		t.Fatalf("root must not start the MCP server:\n%s", text)
	}
}

// Seam: the public CLI tools list is the Archery-only delivery surface.
func TestToolsListContainsOnlyArcheryTools(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--config", archeryConfigPath(t), "tools", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools list: %v", err)
	}
	text := out.String()
	for _, required := range []string{"users.list", "service_2fa.verify", "workflows.submit"} {
		if !strings.Contains(text, required) {
			t.Fatalf("tools list missing %q:\n%s", required, text)
		}
	}
	for _, excluded := range []string{"echo", "ping"} {
		if strings.Contains(text, excluded) {
			t.Fatalf("tools list must not contain %q:\n%s", excluded, text)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "archery-mcp-server") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestToolsUnknownTool(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--config", archeryConfigPath(t), "tools", "describe", "missing-tool"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}

	root = cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--config", archeryConfigPath(t), "tools", "call", "missing-tool", "--params", "{}"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error on call, got %v", err)
	}
}

func TestToolsRejectsNullParams(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--config", archeryConfigPath(t), "tools", "call", "service_2fa.state", "--params", "null"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "expected an object") {
		t.Fatalf("expected JSON object error, got %v", err)
	}
}

func TestToolsCallRejectsSchemaViolationsBeforeInvokingHandler(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	for _, tt := range []struct {
		name   string
		tool   string
		params string
	}{
		{name: "unknown field", tool: "users.create", params: `{"username":"user","password":"secret","display":"User","unexpected":true}`},
		{name: "missing required field", tool: "users.create", params: `{"username":"user"}`},
		{name: "wrong field type", tool: "users.create", params: `{"username":1,"password":"secret","display":"User"}`},
		{name: "missing conditional field", tool: "workflows.execute", params: `{"workflow_id":1,"workflow_type":2}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
			root.SetArgs([]string{"--config", archeryConfigPathForURL(t, upstream.URL), "tools", "call", tt.tool, "--params", tt.params})

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "input schema validation failed") {
				t.Fatalf("tools call error = %v, want input schema validation failure", err)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("upstream requests = %d, want 0", got)
			}
		})
	}
}

func TestCommandsReturnOutputErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name string
		args []string
	}{
		{name: "completion script", args: []string{"completion", "bash"}},
		{name: "completion install", args: []string{"completion", "install"}},
		{name: "version", args: []string{"version"}},
		{name: "tools list", args: []string{"--config", archeryConfigPath(t), "tools", "list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.IOStreams{
				In:     strings.NewReader(""),
				Out:    errorWriter{err: writeErr},
				ErrOut: io.Discard,
			})
			root.SetArgs(tt.args)
			if err := root.Execute(); !errors.Is(err, writeErr) {
				t.Fatalf("Execute() error = %v, want %v", err, writeErr)
			}
		})
	}
}

func archeryConfigPath(t *testing.T) string {
	return archeryConfigPathForURL(t, "https://archery.example.test/prefix")
}

func archeryConfigPathForURL(t *testing.T, baseURL string) string {
	t.Helper()
	t.Setenv("ARCHERY_TEST_PASSWORD", "synthetic-test-value")
	path := t.TempDir() + "/archery.yaml"
	config := "archery:\n  base_url: " + baseURL + "\n  username: mcp-service\n  password_env: ARCHERY_TEST_PASSWORD\n  request_timeout: 15s\n"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
