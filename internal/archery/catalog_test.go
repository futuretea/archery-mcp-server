package archery

import "testing"

// Seam: Operations is the sole fixed method/path catalog observed by callers and tool mappings.
func TestOperationsContainsExactlyTheApprovedArcheryAPIs(t *testing.T) {
	want := []Operation{
		{ID: "users.list", Method: "GET", Path: "/api/v1/user/"},
		{ID: "users.create", Method: "POST", Path: "/api/v1/user/"},
		{ID: "users.update", Method: "PUT", Path: "/api/v1/user/{id}/"},
		{ID: "users.delete", Method: "DELETE", Path: "/api/v1/user/{id}/"},
		{ID: "groups.list", Method: "GET", Path: "/api/v1/user/group/"},
		{ID: "groups.create", Method: "POST", Path: "/api/v1/user/group/"},
		{ID: "groups.update", Method: "PUT", Path: "/api/v1/user/group/{id}/"},
		{ID: "groups.delete", Method: "DELETE", Path: "/api/v1/user/group/{id}/"},
		{ID: "resource_groups.list", Method: "GET", Path: "/api/v1/user/resourcegroup/"},
		{ID: "resource_groups.create", Method: "POST", Path: "/api/v1/user/resourcegroup/"},
		{ID: "resource_groups.update", Method: "PUT", Path: "/api/v1/user/resourcegroup/{id}/"},
		{ID: "resource_groups.delete", Method: "DELETE", Path: "/api/v1/user/resourcegroup/{id}/"},
		{ID: "service_auth.verify", Method: "POST", Path: "/api/v1/user/auth/"},
		{ID: "service_2fa.state", Method: "POST", Path: "/api/v1/user/2fa/state/"},
		{ID: "service_2fa.verify", Method: "POST", Path: "/api/v1/user/2fa/verify/"},
		{ID: "instances.list", Method: "GET", Path: "/api/v1/instance/"},
		{ID: "instances.create", Method: "POST", Path: "/api/v1/instance/"},
		{ID: "instances.update", Method: "PUT", Path: "/api/v1/instance/{id}/"},
		{ID: "instances.delete", Method: "DELETE", Path: "/api/v1/instance/{id}/"},
		{ID: "instances.resources", Method: "POST", Path: "/api/v1/instance/resource/"},
		{ID: "tunnels.list", Method: "GET", Path: "/api/v1/instance/tunnel/"},
		{ID: "tunnels.create", Method: "POST", Path: "/api/v1/instance/tunnel/"},
		{ID: "rds.list", Method: "GET", Path: "/api/v1/instance/rds/"},
		{ID: "rds.create", Method: "POST", Path: "/api/v1/instance/rds/"},
		{ID: "workflows.list", Method: "GET", Path: "/api/v1/workflow/"},
		{ID: "workflows.submit", Method: "POST", Path: "/api/v1/workflow/"},
		{ID: "workflows.sql_check", Method: "POST", Path: "/api/v1/workflow/sqlcheck/"},
		{ID: "workflows.audit", Method: "POST", Path: "/api/v1/workflow/audit/"},
		{ID: "workflows.audit_list", Method: "POST", Path: "/api/v1/workflow/auditlist/"},
		{ID: "workflows.execute", Method: "POST", Path: "/api/v1/workflow/execute/"},
		{ID: "workflows.logs", Method: "POST", Path: "/api/v1/workflow/log/"},
	}

	got := Operations()
	if len(got) != len(want) {
		t.Fatalf("Operations() length = %d, want %d", len(got), len(want))
	}
	byID := make(map[string]Operation, len(got))
	for _, operation := range got {
		if _, duplicate := byID[operation.ID]; duplicate {
			t.Fatalf("Operations() has duplicate ID %q", operation.ID)
		}
		byID[operation.ID] = operation
	}
	for _, expected := range want {
		actual, ok := byID[expected.ID]
		if !ok {
			t.Fatalf("Operations() missing %q", expected.ID)
		}
		if actual.Method != expected.Method || actual.Path != expected.Path {
			t.Fatalf("operation %q = %s %s, want %s %s", expected.ID, actual.Method, actual.Path, expected.Method, expected.Path)
		}
	}
}
