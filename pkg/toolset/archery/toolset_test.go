package archery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	internalarchery "github.com/futuretea/archery-mcp-server/internal/archery"
	"github.com/futuretea/archery-mcp-server/pkg/toolset"
)

// Seam: Mappings is the read-only fixed tool-name to internal-operation catalog.
func TestMappingsContainExactlyOneNamedToolForEveryApprovedOperation(t *testing.T) {
	want := []Mapping{
		{ToolName: "users.list", OperationID: "users.list"},
		{ToolName: "users.create", OperationID: "users.create"},
		{ToolName: "users.update", OperationID: "users.update"},
		{ToolName: "users.delete", OperationID: "users.delete"},
		{ToolName: "groups.list", OperationID: "groups.list"},
		{ToolName: "groups.create", OperationID: "groups.create"},
		{ToolName: "groups.update", OperationID: "groups.update"},
		{ToolName: "groups.delete", OperationID: "groups.delete"},
		{ToolName: "resource_groups.list", OperationID: "resource_groups.list"},
		{ToolName: "resource_groups.create", OperationID: "resource_groups.create"},
		{ToolName: "resource_groups.update", OperationID: "resource_groups.update"},
		{ToolName: "resource_groups.delete", OperationID: "resource_groups.delete"},
		{ToolName: "service_auth.verify", OperationID: "service_auth.verify"},
		{ToolName: "service_2fa.state", OperationID: "service_2fa.state"},
		{ToolName: "service_2fa.verify", OperationID: "service_2fa.verify"},
		{ToolName: "instances.list", OperationID: "instances.list"},
		{ToolName: "instances.create", OperationID: "instances.create"},
		{ToolName: "instances.update", OperationID: "instances.update"},
		{ToolName: "instances.delete", OperationID: "instances.delete"},
		{ToolName: "instances.resources", OperationID: "instances.resources"},
		{ToolName: "tunnels.list", OperationID: "tunnels.list"},
		{ToolName: "tunnels.create", OperationID: "tunnels.create"},
		{ToolName: "rds.list", OperationID: "rds.list"},
		{ToolName: "rds.create", OperationID: "rds.create"},
		{ToolName: "workflows.list", OperationID: "workflows.list"},
		{ToolName: "workflows.submit", OperationID: "workflows.submit"},
		{ToolName: "workflows.sql_check", OperationID: "workflows.sql_check"},
		{ToolName: "workflows.audit", OperationID: "workflows.audit"},
		{ToolName: "workflows.audit_list", OperationID: "workflows.audit_list"},
		{ToolName: "workflows.execute", OperationID: "workflows.execute"},
		{ToolName: "workflows.logs", OperationID: "workflows.logs"},
	}

	got := Mappings()
	if len(got) != len(want) {
		t.Fatalf("Mappings() length = %d, want %d", len(got), len(want))
	}
	operations := make(map[string]struct{}, len(internalarchery.Operations()))
	for _, operation := range internalarchery.Operations() {
		operations[operation.ID] = struct{}{}
	}
	byTool := make(map[string]string, len(got))
	for _, mapping := range got {
		if _, duplicate := byTool[mapping.ToolName]; duplicate {
			t.Fatalf("Mappings() duplicates tool %q", mapping.ToolName)
		}
		if _, exists := operations[mapping.OperationID]; !exists {
			t.Fatalf("Mappings() maps %q to unknown operation %q", mapping.ToolName, mapping.OperationID)
		}
		byTool[mapping.ToolName] = mapping.OperationID
	}
	for _, expected := range want {
		if actual := byTool[expected.ToolName]; actual != expected.OperationID {
			t.Fatalf("mapping %q = %q, want %q", expected.ToolName, actual, expected.OperationID)
		}
	}
	for _, excluded := range []string{"service_2fa.setup", "service_2fa.save"} {
		if _, exists := byTool[excluded]; exists {
			t.Fatalf("Mappings() must exclude %q", excluded)
		}
	}
}

// Seam: Toolset.GetTools exposes the fixed named MCP surface and omits caller-controlled credentials.
func TestToolsetSchemasExcludeCallerIdentityAndTOTPKey(t *testing.T) {
	tools := New(&recordingClient{}, "mcp-service").GetTools()
	if len(tools) != 31 {
		t.Fatalf("GetTools() length = %d, want 31", len(tools))
	}
	for _, registered := range tools {
		properties := schemaProperties(t, registered)
		for _, forbidden := range []string{"engineer", "key", "url", "token", "refresh_token"} {
			if _, exists := properties[forbidden]; exists {
				t.Fatalf("tool %q exposes forbidden parameter %q", registered.Tool.Name, forbidden)
			}
		}
	}
}

// Seam: handlers reject recursive caller identity or TOTP-key inputs before invoking the Archery client.
func TestToolsetRejectsRecursiveEngineerAndKeyBeforeClientCall(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		params map[string]any
		field  string
	}{
		{
			name: "nested engineer",
			tool: "workflows.submit",
			params: map[string]any{
				"is_offline_export": true,
				"workflow":          map[string]any{"engineer": "caller-controlled"},
			},
			field: "engineer",
		},
		{
			name: "nested key",
			tool: "service_2fa.verify",
			params: map[string]any{
				"auth_type": "totp",
				"otp":       123456,
				"metadata":  map[string]any{"key": "caller-controlled"},
			},
			field: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingClient{}
			handler := toolHandler(t, New(client, "mcp-service").GetTools(), tt.tool)
			_, err := handler(context.Background(), tt.params)
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("handler error = %v, want rejected %q input", err, tt.field)
			}
			if len(client.calls) != 0 {
				t.Fatalf("client calls = %d, want 0 after rejected input", len(client.calls))
			}
		})
	}
}

// Seam: a handler injects the configured service account and returns redacted upstream JSON.
func TestToolsetInjectsServiceAccountAndRedactsUpstreamJSON(t *testing.T) {
	client := &recordingClient{response: json.RawMessage(`{"status":"visible","token":"synthetic-sensitive","nested":{"password":"synthetic-sensitive"}}`)}
	handler := toolHandler(t, New(client, "mcp-service").GetTools(), "service_2fa.state")

	result, err := handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("client calls = %d, want 1", len(client.calls))
	}
	if got := client.calls[0].body["engineer"]; got != "mcp-service" {
		t.Fatalf("injected engineer = %v, want configured service account", got)
	}
	if !strings.Contains(result, "visible") {
		t.Fatalf("handler result = %q, want retained non-sensitive data", result)
	}
	if strings.Contains(result, "synthetic-sensitive") {
		t.Fatalf("handler result leaked sensitive upstream value: %q", result)
	}
}

func TestToolsetConvertsListCreateAndUpdateInputs(t *testing.T) {
	runConversionCases(t, []conversionCase{
		{
			name:   "list",
			tool:   "users.list",
			params: map[string]any{"page": 2, "size": 50},
			assertCall: func(t *testing.T, call recordedCall) {
				if call.operation.Method != http.MethodGet || call.query.Encode() != "page=2&size=50" || call.query.Has("page_size") || call.body != nil {
					t.Fatalf("list conversion = method %s query %v body %v", call.operation.Method, call.query, call.body)
				}
			},
		},
		{
			name:   "create",
			tool:   "groups.create",
			params: map[string]any{"name": "operators"},
			assertCall: func(t *testing.T, call recordedCall) {
				if call.body["name"] != "operators" || len(call.body) != 1 {
					t.Fatalf("create body = %v", call.body)
				}
			},
		},
		{
			name:   "update",
			tool:   "users.update",
			params: map[string]any{"id": 7, "display": "Operator"},
			assertCall: func(t *testing.T, call recordedCall) {
				if call.pathParams["id"] != "7" || call.body["display"] != "Operator" || call.body["id"] != nil {
					t.Fatalf("update conversion = path %v body %v", call.pathParams, call.body)
				}
			},
		},
	})
}

func TestToolsetConvertsWorkflowAndTwoFactorInputs(t *testing.T) {
	runConversionCases(t, []conversionCase{
		{
			name: "workflow submit",
			tool: "workflows.submit",
			params: map[string]any{
				"sql_content": "select 1",
				"workflow": map[string]any{
					"workflow_name":     "check",
					"group_id":          1,
					"instance":          2,
					"db_name":           "app",
					"is_offline_export": false,
				},
			},
			assertCall: func(t *testing.T, call recordedCall) {
				workflow, ok := call.body["workflow"].(map[string]any)
				if !ok || workflow["engineer"] != "mcp-service" || call.body["engineer"] != nil {
					t.Fatalf("workflow submit body = %v", call.body)
				}
			},
		},
		{
			name:   "two factor verification",
			tool:   "service_2fa.verify",
			params: map[string]any{"auth_type": "totp", "otp": 123456},
			assertCall: func(t *testing.T, call recordedCall) {
				if call.body["auth_type"] != "totp" || call.body["otp"] != 123456 || call.body["engineer"] != "mcp-service" {
					t.Fatalf("2FA body = %v", call.body)
				}
			},
		},
	})
}

// Seam: the public workflows.audit_list schema and handler preserve Archery pagination in its query.
func TestToolsetWorkflowAuditListPaginationIsQueryScoped(t *testing.T) {
	client := &recordingClient{}
	tools := New(client, "mcp-service").GetTools()
	handler := toolHandler(t, tools, "workflows.audit_list")
	var registered toolset.ServerTool
	for _, candidate := range tools {
		if candidate.Tool.Name == "workflows.audit_list" {
			registered = candidate
			break
		}
	}

	properties := schemaProperties(t, registered)
	for _, field := range []string{"page", "size"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("workflows.audit_list schema misses pagination field %q", field)
		}
	}
	if _, ok := properties["page_size"]; ok {
		t.Fatal("workflows.audit_list schema exposes unsupported page_size")
	}

	if _, err := handler(context.Background(), map[string]any{"page": 2, "size": 50}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("client calls = %d, want 1", len(client.calls))
	}
	call := client.calls[0]
	if call.query.Encode() != "page=2&size=50" || len(call.body) != 1 || call.body["engineer"] != "mcp-service" {
		t.Fatalf("audit-list call = query %v body %v, want query page=2&size=50 and service account only", call.query, call.body)
	}
}

// Seam: the public workflows.logs schema and handler preserve Archery pagination in its query.
func TestToolsetWorkflowLogsPaginationIsQueryScoped(t *testing.T) {
	client := &recordingClient{}
	tools := New(client, "mcp-service").GetTools()
	handler := toolHandler(t, tools, "workflows.logs")
	var registered toolset.ServerTool
	for _, candidate := range tools {
		if candidate.Tool.Name == "workflows.logs" {
			registered = candidate
			break
		}
	}

	properties := schemaProperties(t, registered)
	for _, field := range []string{"workflow_id", "workflow_type", "page", "size"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("workflows.logs schema misses field %q", field)
		}
	}

	if _, err := handler(context.Background(), map[string]any{"workflow_id": 7, "workflow_type": 2, "page": 2, "size": 50}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("client calls = %d, want 1", len(client.calls))
	}
	call := client.calls[0]
	if call.query.Encode() != "page=2&size=50" || len(call.body) != 2 || call.body["workflow_id"] != 7 || call.body["workflow_type"] != 2 || call.body["page"] != nil || call.body["size"] != nil || call.body["engineer"] != nil {
		t.Fatalf("workflow-logs call = query %v body %v, want query page=2&size=50 and workflow identity only", call.query, call.body)
	}
}

type conversionCase struct {
	name       string
	tool       string
	params     map[string]any
	assertCall func(*testing.T, recordedCall)
}

func runConversionCases(t *testing.T, tests []conversionCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingClient{}
			handler := toolHandler(t, New(client, "mcp-service").GetTools(), tt.tool)
			if _, err := handler(context.Background(), tt.params); err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if len(client.calls) != 1 {
				t.Fatalf("client calls = %d, want 1", len(client.calls))
			}
			tt.assertCall(t, client.calls[0])
		})
	}
}

type recordingClient struct {
	calls    []recordedCall
	response json.RawMessage
}

type recordedCall struct {
	operation  internalarchery.Operation
	pathParams map[string]string
	query      url.Values
	body       map[string]any
}

func (c *recordingClient) Do(_ context.Context, operation internalarchery.Operation, pathParams map[string]string, query url.Values, body map[string]any) (json.RawMessage, error) {
	c.calls = append(c.calls, recordedCall{operation: operation, pathParams: pathParams, query: query, body: body})
	if c.response == nil {
		return json.RawMessage(`{}`), nil
	}
	return c.response, nil
}

func toolHandler(t *testing.T, tools []toolset.ServerTool, name string) toolset.ToolHandler {
	t.Helper()
	for _, registered := range tools {
		if registered.Tool.Name == name {
			return registered.Handler
		}
	}
	t.Fatalf("GetTools() missing %q", name)
	return nil
}

func schemaProperties(t *testing.T, registered toolset.ServerTool) map[string]json.RawMessage {
	t.Helper()
	raw := registered.Tool.RawInputSchema
	if raw == nil {
		var err error
		raw, err = json.Marshal(registered.Tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", registered.Tool.Name, err)
		}
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema for %q: %v", registered.Tool.Name, err)
	}
	return schema.Properties
}
