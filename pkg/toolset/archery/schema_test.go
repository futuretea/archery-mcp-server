package archery

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/futuretea/archery-mcp-server/pkg/toolset"
)

type schemaExpectation struct {
	fields   []string
	required []string
}

type parsedInputSchema struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
	AllOf      []map[string]any           `json:"allOf"`
}

func TestToolsetSchemasMatchFixedOperationMatrix(t *testing.T) {
	expected := map[string]schemaExpectation{
		"users.list":             {fields: []string{"page", "size"}},
		"users.create":           {fields: []string{"display", "email", "groups", "password", "resource_group", "username"}, required: []string{"display", "password", "username"}},
		"users.update":           {fields: []string{"display", "email", "groups", "id", "password", "resource_group"}, required: []string{"id"}},
		"users.delete":           {fields: []string{"id"}, required: []string{"id"}},
		"groups.list":            {fields: []string{"page", "size"}},
		"groups.create":          {fields: []string{"name"}, required: []string{"name"}},
		"groups.update":          {fields: []string{"id", "name"}, required: []string{"id"}},
		"groups.delete":          {fields: []string{"id"}, required: []string{"id"}},
		"resource_groups.list":   {fields: []string{"page", "size"}},
		"resource_groups.create": {fields: []string{"ding_webhook", "feishu_webhook", "group_level", "group_name", "qywx_webhook"}, required: []string{"group_name"}},
		"resource_groups.update": {fields: []string{"ding_webhook", "feishu_webhook", "group_level", "group_name", "id", "qywx_webhook"}, required: []string{"id"}},
		"resource_groups.delete": {fields: []string{"id"}, required: []string{"id"}},
		"service_auth.verify":    {},
		"service_2fa.state":      {},
		"service_2fa.verify":     {fields: []string{"auth_type", "otp", "phone"}, required: []string{"auth_type", "otp"}},
		"instances.list":         {fields: []string{"page", "size"}},
		"instances.create":       {fields: []string{"charset", "db_name", "db_type", "denied_db_name_regex", "host", "instance_name", "instance_tag", "is_ssl", "mode", "password", "port", "resource_group", "service_name", "show_db_name_regex", "sid", "tunnel", "type", "user", "verify_ssl"}, required: []string{"db_type", "host", "instance_name", "type"}},
		"instances.update":       {fields: []string{"charset", "db_name", "db_type", "denied_db_name_regex", "host", "id", "instance_name", "instance_tag", "is_ssl", "mode", "password", "port", "resource_group", "service_name", "show_db_name_regex", "sid", "tunnel", "type", "user", "verify_ssl"}, required: []string{"id"}},
		"instances.delete":       {fields: []string{"id"}, required: []string{"id"}},
		"instances.resources":    {fields: []string{"db_name", "instance_id", "resource_type", "schema_name", "tb_name"}, required: []string{"instance_id", "resource_type"}},
		"tunnels.list":           {fields: []string{"page", "size"}},
		"tunnels.create":         {fields: []string{"host", "password", "pkey", "pkey_password", "port", "tunnel_name", "user"}, required: []string{"host", "tunnel_name"}},
		"rds.list":               {fields: []string{"page", "size"}},
		"rds.create":             {fields: []string{"ak", "instance", "is_enable", "rds_dbinstanceid"}, required: []string{"ak", "instance", "rds_dbinstanceid"}},
		"workflows.list":         {fields: []string{"page", "size"}},
		"workflows.submit":       {fields: []string{"sql_content", "workflow"}, required: []string{"sql_content", "workflow"}},
		"workflows.sql_check":    {fields: []string{"db_name", "full_sql", "instance_id"}, required: []string{"db_name", "full_sql", "instance_id"}},
		"workflows.audit":        {fields: []string{"audit_remark", "audit_type", "workflow_id", "workflow_type"}, required: []string{"audit_remark", "audit_type", "workflow_id", "workflow_type"}},
		"workflows.audit_list":   {fields: []string{"page", "size"}},
		"workflows.execute":      {fields: []string{"mode", "workflow_id", "workflow_type"}, required: []string{"workflow_id", "workflow_type"}},
		"workflows.logs":         {fields: []string{"page", "size", "workflow_id", "workflow_type"}, required: []string{"workflow_id", "workflow_type"}},
	}

	tools := New(&recordingClient{}, "mcp-service").GetTools()
	if len(tools) != len(expected) {
		t.Fatalf("GetTools() length = %d, want %d", len(tools), len(expected))
	}
	for _, registered := range tools {
		want, ok := expected[registered.Tool.Name]
		if !ok {
			t.Fatalf("unexpected tool schema %q", registered.Tool.Name)
		}
		got := decodeInputSchema(t, registered)
		assertSchemaFields(t, registered.Tool.Name, got, want)
	}
}

func TestToolsetKeepsCloudAccessKeyInputsDistinctFromLiteralKey(t *testing.T) {
	client := &recordingClient{response: json.RawMessage(`{"results":[{"ak":{"key_id":"visible-id","key_secret":"visible-secret"}}]}`)}
	tools := New(client, "mcp-service").GetTools()

	create := toolHandler(t, tools, "rds.create")
	if _, err := create(context.Background(), map[string]any{
		"rds_dbinstanceid": "db-1",
		"instance":         1,
		"ak":               map[string]any{"key_id": "input-id", "key_secret": "input-secret"},
	}); err != nil {
		t.Fatalf("rds.create rejected legitimate ak.key_id input: %v", err)
	}

	list := toolHandler(t, tools, "rds.list")
	result, err := list(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("rds.list error = %v", err)
	}
	if strings.Contains(result, "visible-id") || strings.Contains(result, "visible-secret") {
		t.Fatalf("redacted RDS result leaked cloud credentials: %s", result)
	}
	if strings.Count(result, "[REDACTED]") != 2 {
		t.Fatalf("redacted RDS result = %s, want key_id and key_secret redacted", result)
	}
}

func TestToolsetSchemasRequireArcheryConditionalInputs(t *testing.T) {
	tests := []struct {
		tool     string
		field    string
		value    any
		required string
	}{
		{tool: "service_2fa.verify", field: "auth_type", value: "sms", required: "phone"},
		{tool: "workflows.execute", field: "workflow_type", value: float64(2), required: "mode"},
	}
	tools := New(&recordingClient{}, "mcp-service").GetTools()
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			schema := decodeInputSchema(t, mustFindTool(t, tools, tt.tool))
			if len(schema.AllOf) != 1 {
				t.Fatalf("%s conditional rules = %d, want 1", tt.tool, len(schema.AllOf))
			}
			condition := schema.AllOf[0]
			ifFields, ok := condition["if"].(map[string]any)
			if !ok {
				t.Fatalf("%s allOf.if = %T, want object", tt.tool, condition["if"])
			}
			properties := ifFields["properties"].(map[string]any)
			field := properties[tt.field].(map[string]any)
			if field["const"] != tt.value {
				t.Fatalf("%s condition %s = %v, want %v", tt.tool, tt.field, field["const"], tt.value)
			}
			then := condition["then"].(map[string]any)
			required := then["required"].([]any)
			if len(required) != 1 || required[0] != tt.required {
				t.Fatalf("%s conditional required = %v, want %q", tt.tool, required, tt.required)
			}
		})
	}
}

func decodeInputSchema(t *testing.T, registered toolset.ServerTool) parsedInputSchema {
	t.Helper()
	raw := registered.Tool.RawInputSchema
	if raw == nil {
		var err error
		raw, err = json.Marshal(registered.Tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", registered.Tool.Name, err)
		}
	}
	var schema parsedInputSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema for %q: %v", registered.Tool.Name, err)
	}
	return schema
}

func mustFindTool(t *testing.T, tools []toolset.ServerTool, name string) toolset.ServerTool {
	t.Helper()
	for _, registered := range tools {
		if registered.Tool.Name == name {
			return registered
		}
	}
	t.Fatalf("GetTools() missing %q", name)
	return toolset.ServerTool{}
}

func assertSchemaFields(t *testing.T, name string, got parsedInputSchema, want schemaExpectation) {
	t.Helper()
	gotFields := make([]string, 0, len(got.Properties))
	for field := range got.Properties {
		gotFields = append(gotFields, field)
	}
	sort.Strings(gotFields)
	sort.Strings(want.fields)
	sort.Strings(got.Required)
	sort.Strings(want.required)
	if strings.Join(gotFields, ",") != strings.Join(want.fields, ",") {
		t.Fatalf("tool %q fields = %v, want %v", name, gotFields, want.fields)
	}
	if strings.Join(got.Required, ",") != strings.Join(want.required, ",") {
		t.Fatalf("tool %q required = %v, want %v", name, got.Required, want.required)
	}
	for _, forbidden := range []string{"engineer", "key", "url", "token", "refresh_token"} {
		if _, exists := got.Properties[forbidden]; exists {
			t.Fatalf("tool %q exposes forbidden parameter %q", name, forbidden)
		}
	}
}
