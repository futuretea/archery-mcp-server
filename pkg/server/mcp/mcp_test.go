package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/futuretea/archery-mcp-server/pkg/core/config"
	mcpserver "github.com/futuretea/archery-mcp-server/pkg/server/mcp"
	"github.com/futuretea/archery-mcp-server/pkg/toolset"
	"github.com/mark3labs/mcp-go/mcp"
)

type testToolset struct{}

func (testToolset) GetName() string        { return "test" }
func (testToolset) GetDescription() string { return "test tools" }
func (testToolset) GetTools() []toolset.ServerTool {
	return []toolset.ServerTool{{Tool: mcp.NewTool("test.tool"), Domain: "test", Handler: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }}}
}

func TestNewServerRegistersTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{testToolset{}},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	if !server.IsHealthy() {
		t.Fatal("expected healthy server")
	}
	tools := server.GetEnabledTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", tools)
	}
	names := map[string]bool{}
	for _, name := range tools {
		names[name] = true
	}
	if !names["test.tool"] {
		t.Fatalf("expected test tool, got %v", tools)
	}
}

func TestNewServerRequiresStaticConfig(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{})
	if err == nil || !strings.Contains(err.Error(), "static config is required") {
		t.Fatalf("expected static config error, got %v", err)
	}
}

func TestNewServerRejectsEmptyToolFilter(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{
			LogLevel:     "info",
			EnabledTools: []string{"missing"},
		},
		Toolsets: []toolset.Toolset{testToolset{}},
	})
	if err == nil || !strings.Contains(err.Error(), "no tools registered") {
		t.Fatalf("expected no tools registered error, got %v", err)
	}
}

func TestNewServerRejectsDuplicateToolNames(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets: []toolset.Toolset{
			testToolset{},
			testToolset{},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate tool name "test.tool"`) {
		t.Fatalf("expected duplicate tool name error, got %v", err)
	}
}

func TestNewTextResult(t *testing.T) {
	ok := mcpserver.NewTextResult("hello", nil)
	if ok.IsError || len(ok.Content) != 1 {
		t.Fatalf("unexpected ok result: %+v", ok)
	}

	fail := mcpserver.NewTextResult("", errString("boom"))
	if !fail.IsError {
		t.Fatal("expected error result")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
