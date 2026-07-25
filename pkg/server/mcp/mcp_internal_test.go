package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	internalarchery "github.com/futuretea/archery-mcp-server/internal/archery"
	"github.com/futuretea/archery-mcp-server/pkg/core/config"
	archerytoolset "github.com/futuretea/archery-mcp-server/pkg/toolset/archery"
	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/archery-mcp-server/pkg/toolset"
)

func TestNewToolHandlerReturnsErrorResult(t *testing.T) {
	wantErr := errors.New("boom")
	handler := newToolHandler(toolset.ServerTool{
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			if params == nil {
				t.Fatal("handler params must be a non-nil map")
			}
			return "", wantErr
		},
	})

	result, err := handler(t.Context(), mcpprotocol.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler transport error = %v, want nil", err)
	}
	if !result.IsError {
		t.Fatal("handler result IsError = false, want true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("handler content count = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(mcpprotocol.TextContent)
	if !ok {
		t.Fatalf("handler content type = %T, want mcp.TextContent", result.Content[0])
	}
	if text.Type != "text" || text.Text != wantErr.Error() {
		t.Fatalf("handler text = {Type:%q Text:%q}, want {Type:%q Text:%q}", text.Type, text.Text, "text", wantErr)
	}
}

func TestNewServerValidatesArcherySchemasBeforeHandlers(t *testing.T) {
	client := &schemaValidationClient{}
	server, err := NewServer(Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{archerytoolset.New(client, "mcp-service")},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	invalid := []struct {
		name string
		args map[string]any
	}{
		{name: "unknown top level", args: map[string]any{"username": "user", "password": "secret", "display": "User", "unexpected": true}},
		{name: "missing required", args: map[string]any{"username": "user"}},
		{name: "wrong type", args: map[string]any{"username": 1, "password": "secret", "display": "User"}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if result := callSchemaTool(t, server, "users.create", tt.args); !result.IsError {
				t.Fatalf("invalid call result = %#v, want tool error", result)
			}
			if client.calls != 0 {
				t.Fatalf("handler calls = %d, want 0", client.calls)
			}
		})
	}

	for _, tt := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "unknown nested", tool: "rds.create", args: map[string]any{"rds_dbinstanceid": "db", "instance": 1, "ak": map[string]any{"key_id": "id", "key_secret": "secret", "unexpected": true}}},
		{name: "SMS phone required", tool: "service_2fa.verify", args: map[string]any{"auth_type": "sms", "otp": 123456}},
		{name: "type two execution mode required", tool: "workflows.execute", args: map[string]any{"workflow_id": 1, "workflow_type": 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if result := callSchemaTool(t, server, tt.tool, tt.args); !result.IsError {
				t.Fatalf("invalid call result = %#v, want tool error", result)
			}
			if client.calls != 0 {
				t.Fatalf("handler calls = %d, want 0", client.calls)
			}
		})
	}

	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{tool: "service_2fa.verify", args: map[string]any{"auth_type": "sms", "otp": 123456, "phone": "13800000000"}},
		{tool: "workflows.execute", args: map[string]any{"workflow_id": 1, "workflow_type": 2, "mode": "auto"}},
	} {
		if result := callSchemaTool(t, server, tt.tool, tt.args); result.IsError {
			t.Fatalf("valid %s result = %#v, want success", tt.tool, result)
		}
	}
	if client.calls != 2 {
		t.Fatalf("handler calls = %d, want 2 valid calls", client.calls)
	}
}

type schemaValidationClient struct {
	calls int
}

func (c *schemaValidationClient) Do(_ context.Context, _ internalarchery.Operation, _ map[string]string, _ url.Values, _ map[string]any) (json.RawMessage, error) {
	c.calls++
	return json.RawMessage(`{}`), nil
}

func callSchemaTool(t *testing.T, server *Server, name string, args map[string]any) *mcpprotocol.CallToolResult {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatalf("marshal tools/call: %v", err)
	}
	response := server.server.HandleMessage(t.Context(), raw)
	jsonResponse, ok := response.(mcpprotocol.JSONRPCResponse)
	if !ok {
		t.Fatalf("tools/call response = %T, want JSONRPCResponse", response)
	}
	result, ok := jsonResponse.Result.(*mcpprotocol.CallToolResult)
	if !ok {
		t.Fatalf("tools/call result = %T, want CallToolResult", jsonResponse.Result)
	}
	return result
}
