package archery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	internalarchery "github.com/futuretea/archery-mcp-server/internal/archery"
	"github.com/futuretea/archery-mcp-server/pkg/toolset"
	"github.com/mark3labs/mcp-go/mcp"
)

type Client interface {
	Do(context.Context, internalarchery.Operation, map[string]string, url.Values, map[string]any) (json.RawMessage, error)
}

type Toolset struct {
	client         Client
	serviceAccount string
}
type Mapping struct {
	ToolName    string
	OperationID string
}

func New(client Client, serviceAccount string) *Toolset {
	return &Toolset{client: client, serviceAccount: serviceAccount}
}
func (t *Toolset) GetName() string        { return "archery" }
func (t *Toolset) GetDescription() string { return "Fixed Archery 1.14.0 API tools" }

func Mappings() []Mapping {
	operations := internalarchery.Operations()
	mappings := make([]Mapping, len(operations))
	for i, operation := range operations {
		mappings[i] = Mapping{ToolName: operation.ID, OperationID: operation.ID}
	}
	return mappings
}

func (t *Toolset) GetTools() []toolset.ServerTool {
	operations := make(map[string]internalarchery.Operation)
	for _, operation := range internalarchery.Operations() {
		operations[operation.ID] = operation
	}
	tools := make([]toolset.ServerTool, 0, len(operations))
	for _, mapping := range Mappings() {
		operation := operations[mapping.OperationID]
		tools = append(tools, toolset.ServerTool{Tool: mcp.NewToolWithRawSchema(mapping.ToolName, "Call fixed Archery operation "+mapping.OperationID, inputSchema(mapping.OperationID)), Domain: "archery", Handler: t.handler(operation)})
	}
	return tools
}

func (t *Toolset) handler(operation internalarchery.Operation) toolset.ToolHandler {
	return func(ctx context.Context, params map[string]any) (string, error) {
		if field, found := forbiddenField(params); found {
			return "", fmt.Errorf("caller-provided %s is not allowed", field)
		}
		body := clone(params)
		if operation.ID == "service_2fa.verify" {
			authType, _ := body["auth_type"].(string)
			if authType != "totp" && authType != "sms" {
				return "", fmt.Errorf("auth_type must be totp or sms")
			}
		}
		if operation.ID == "workflows.submit" {
			workflow, _ := body["workflow"].(map[string]any)
			if workflow == nil {
				return "", fmt.Errorf("workflow is required")
			}
			if _, ok := workflow["is_offline_export"]; !ok {
				return "", fmt.Errorf("workflow.is_offline_export is required")
			}
		}
		pathParams := map[string]string{}
		if strings.Contains(operation.Path, "{id}") {
			if value, ok := body["id"]; ok {
				pathParams["id"] = fmt.Sprint(value)
				delete(body, "id")
			} else {
				return "", fmt.Errorf("id is required")
			}
		}
		query := url.Values{}
		if operation.ID == "workflows.audit_list" || operation.ID == "workflows.logs" {
			for _, key := range []string{"page", "size"} {
				if value, ok := body[key]; ok {
					query.Set(key, fmt.Sprint(value))
					delete(body, key)
				}
			}
		}
		if operation.Method == "GET" {
			for key, value := range body {
				query.Set(key, fmt.Sprint(value))
			}
			body = nil
		}
		if operation.ID == "workflows.submit" {
			workflow := clone(body["workflow"].(map[string]any))
			workflow["engineer"] = t.serviceAccount
			body["workflow"] = workflow
		} else if needsServiceAccount(operation.ID) {
			if body == nil {
				body = map[string]any{}
			}
			body["engineer"] = t.serviceAccount
		}
		response, err := t.client.Do(ctx, operation, pathParams, query, body)
		if err != nil {
			return "", err
		}
		return redactJSON(response), nil
	}
}

func needsServiceAccount(operation string) bool {
	return operation == "service_auth.verify" || operation == "service_2fa.state" || operation == "service_2fa.verify" || strings.HasPrefix(operation, "workflows.") && operation != "workflows.sql_check" && operation != "workflows.logs"
}
func forbiddenField(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "engineer" || key == "key" {
				return key, true
			}
			if field, found := forbiddenField(child); found {
				return field, true
			}
		}
	case []any:
		for _, child := range typed {
			if field, found := forbiddenField(child); found {
				return field, true
			}
		}
	}
	return "", false
}
func clone(values map[string]any) map[string]any {
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
func redactJSON(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "{}"
	}
	redact(value)
	out, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(out)
}
func redact(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if sensitive(key) {
				typed[key] = "[REDACTED]"
			} else {
				redact(child)
			}
		}
	case []any:
		for _, child := range typed {
			redact(child)
		}
	}
}
func sensitive(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret") || key == "key" || key == "key_id" || strings.Contains(key, "otp") || strings.Contains(key, "pkey")
}
