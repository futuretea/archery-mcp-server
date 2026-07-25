package archery

import (
	"encoding/json"
)

type schemaField struct {
	name     string
	schema   map[string]any
	required bool
}

var operationSchemas = map[string]json.RawMessage{
	"users.list":             schema(pageFields()...),
	"users.create":           schema(stringField("username", true), stringField("password", true), stringField("display", true), stringField("email", false), integerArrayField("groups", false), integerArrayField("resource_group", false)),
	"users.update":           schema(integerField("id", true), stringField("password", false), stringField("display", false), stringField("email", false), integerArrayField("groups", false), integerArrayField("resource_group", false)),
	"users.delete":           schema(integerField("id", true)),
	"groups.list":            schema(pageFields()...),
	"groups.create":          schema(stringField("name", true)),
	"groups.update":          schema(integerField("id", true), stringField("name", false)),
	"groups.delete":          schema(integerField("id", true)),
	"resource_groups.list":   schema(pageFields()...),
	"resource_groups.create": schema(stringField("group_name", true), integerField("group_level", false), stringField("ding_webhook", false), stringField("feishu_webhook", false), stringField("qywx_webhook", false)),
	"resource_groups.update": schema(integerField("id", true), stringField("group_name", false), integerField("group_level", false), stringField("ding_webhook", false), stringField("feishu_webhook", false), stringField("qywx_webhook", false)),
	"resource_groups.delete": schema(integerField("id", true)),
	"service_auth.verify":    schema(),
	"service_2fa.state":      schema(),
	"service_2fa.verify":     schemaWithConditions([]schemaField{enumField("auth_type", true, "totp", "sms"), integerField("otp", true), stringField("phone", false)}, requiredWhen("auth_type", "sms", "phone")),
	"instances.list":         schema(pageFields()...),
	"instances.create":       instanceSchema(false),
	"instances.update":       instanceSchema(true),
	"instances.delete":       schema(integerField("id", true)),
	"instances.resources":    schema(integerField("instance_id", true), enumField("resource_type", true, "database", "schema", "table", "column"), stringField("db_name", false), stringField("schema_name", false), stringField("tb_name", false)),
	"tunnels.list":           schema(pageFields()...),
	"tunnels.create":         schema(stringField("tunnel_name", true), stringField("host", true), integerField("port", false), stringField("user", false), stringField("password", false), stringField("pkey", false), stringField("pkey_password", false)),
	"rds.list":               schema(pageFields()...),
	"rds.create":             schema(stringField("rds_dbinstanceid", true), booleanField("is_enable", false), integerField("instance", true), objectField("ak", true, stringField("type", false), stringField("key_id", true), stringField("key_secret", true))),
	"workflows.list":         schema(pageFields()...),
	"workflows.submit":       schema(stringField("sql_content", true), objectField("workflow", true, stringField("workflow_name", true), integerField("group_id", true), integerField("instance", true), stringField("db_name", true), booleanField("is_offline_export", true), booleanField("is_backup", false), stringField("demand_url", false))),
	"workflows.sql_check":    schema(integerField("instance_id", true), stringField("db_name", true), stringField("full_sql", true)),
	"workflows.audit":        schema(integerField("workflow_id", true), stringField("audit_remark", true), integerEnumField("workflow_type", true, 1, 2, 3), enumField("audit_type", true, "pass", "cancel")),
	"workflows.audit_list":   schema(pageFields()...),
	"workflows.execute":      schemaWithConditions([]schemaField{integerField("workflow_id", true), integerEnumField("workflow_type", true, 2, 3), enumField("mode", false, "auto", "manual")}, requiredWhen("workflow_type", 2, "mode")),
	"workflows.logs":         schema(integerField("workflow_id", true), integerEnumField("workflow_type", true, 1, 2, 3), integerField("page", false), integerField("size", false)),
}

func inputSchema(operationID string) json.RawMessage {
	schema, ok := operationSchemas[operationID]
	if !ok {
		panic("missing fixed schema for " + operationID)
	}
	return schema
}

func pageFields() []schemaField {
	return []schemaField{integerField("page", false), integerField("size", false)}
}

func instanceSchema(withID bool) json.RawMessage {
	fields := []schemaField{
		stringField("instance_name", !withID), enumField("type", !withID, "master", "slave"), stringField("db_type", !withID), stringField("host", !withID),
		integerField("port", false), stringField("user", false), stringField("password", false), booleanField("is_ssl", false), booleanField("verify_ssl", false),
		stringField("db_name", false), stringField("show_db_name_regex", false), stringField("denied_db_name_regex", false), stringField("charset", false),
		stringField("service_name", false), stringField("sid", false), enumField("mode", false, "standalone", "cluster"), integerArrayField("resource_group", false), integerArrayField("instance_tag", false), integerField("tunnel", false),
	}
	if withID {
		fields = append([]schemaField{integerField("id", true)}, fields...)
	}
	return schema(fields...)
}

func schema(fields ...schemaField) json.RawMessage {
	return schemaWithConditions(fields)
}

func schemaWithConditions(fields []schemaField, conditions ...map[string]any) json.RawMessage {
	properties, required := objectSchema(fields...)
	definition := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	if len(conditions) > 0 {
		definition["allOf"] = conditions
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		panic(err)
	}
	return raw
}

func requiredWhen(field string, value any, required string) map[string]any {
	return map[string]any{
		"if":   map[string]any{"properties": map[string]any{field: map[string]any{"const": value}}, "required": []string{field}},
		"then": map[string]any{"required": []string{required}},
	}
}

func objectSchema(fields ...schemaField) (map[string]any, []string) {
	properties := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		properties[field.name] = field.schema
		if field.required {
			required = append(required, field.name)
		}
	}
	return properties, required
}

func stringField(name string, required bool) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "string"}, required: required}
}

func integerField(name string, required bool) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "integer"}, required: required}
}

func booleanField(name string, required bool) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "boolean"}, required: required}
}

func enumField(name string, required bool, values ...string) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "string", "enum": values}, required: required}
}

func integerEnumField(name string, required bool, values ...int) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "integer", "enum": values}, required: required}
}

func integerArrayField(name string, required bool) schemaField {
	return schemaField{name: name, schema: map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, required: required}
}

func objectField(name string, required bool, fields ...schemaField) schemaField {
	properties, requiredFields := objectSchema(fields...)
	return schemaField{name: name, schema: map[string]any{"type": "object", "properties": properties, "required": requiredFields, "additionalProperties": false}, required: required}
}
