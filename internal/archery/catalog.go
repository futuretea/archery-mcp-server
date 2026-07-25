package archery

import "net/http"

type Operation struct {
	ID     string
	Method string
	Path   string
}

func Operations() []Operation {
	return []Operation{
		{"users.list", http.MethodGet, "/api/v1/user/"}, {"users.create", http.MethodPost, "/api/v1/user/"}, {"users.update", http.MethodPut, "/api/v1/user/{id}/"}, {"users.delete", http.MethodDelete, "/api/v1/user/{id}/"},
		{"groups.list", http.MethodGet, "/api/v1/user/group/"}, {"groups.create", http.MethodPost, "/api/v1/user/group/"}, {"groups.update", http.MethodPut, "/api/v1/user/group/{id}/"}, {"groups.delete", http.MethodDelete, "/api/v1/user/group/{id}/"},
		{"resource_groups.list", http.MethodGet, "/api/v1/user/resourcegroup/"}, {"resource_groups.create", http.MethodPost, "/api/v1/user/resourcegroup/"}, {"resource_groups.update", http.MethodPut, "/api/v1/user/resourcegroup/{id}/"}, {"resource_groups.delete", http.MethodDelete, "/api/v1/user/resourcegroup/{id}/"},
		{"service_auth.verify", http.MethodPost, "/api/v1/user/auth/"}, {"service_2fa.state", http.MethodPost, "/api/v1/user/2fa/state/"}, {"service_2fa.verify", http.MethodPost, "/api/v1/user/2fa/verify/"},
		{"instances.list", http.MethodGet, "/api/v1/instance/"}, {"instances.create", http.MethodPost, "/api/v1/instance/"}, {"instances.update", http.MethodPut, "/api/v1/instance/{id}/"}, {"instances.delete", http.MethodDelete, "/api/v1/instance/{id}/"}, {"instances.resources", http.MethodPost, "/api/v1/instance/resource/"},
		{"tunnels.list", http.MethodGet, "/api/v1/instance/tunnel/"}, {"tunnels.create", http.MethodPost, "/api/v1/instance/tunnel/"}, {"rds.list", http.MethodGet, "/api/v1/instance/rds/"}, {"rds.create", http.MethodPost, "/api/v1/instance/rds/"},
		{"workflows.list", http.MethodGet, "/api/v1/workflow/"}, {"workflows.submit", http.MethodPost, "/api/v1/workflow/"}, {"workflows.sql_check", http.MethodPost, "/api/v1/workflow/sqlcheck/"}, {"workflows.audit", http.MethodPost, "/api/v1/workflow/audit/"}, {"workflows.audit_list", http.MethodPost, "/api/v1/workflow/auditlist/"}, {"workflows.execute", http.MethodPost, "/api/v1/workflow/execute/"}, {"workflows.logs", http.MethodPost, "/api/v1/workflow/log/"},
	}
}
