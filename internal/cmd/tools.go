package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/futuretea/archery-mcp-server/pkg/toolset"
)

func newToolsCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	command := &cobra.Command{
		Use:     "tools",
		Aliases: []string{"tool"},
		Short:   "list, describe, and call MCP tools",
	}
	command.AddCommand(newToolsListCommand(streams, cfgFile, v))
	command.AddCommand(newToolsDescribeCommand(streams, cfgFile, v))
	command.AddCommand(newToolsCallCommand(streams, cfgFile, v))
	return command
}

func newToolsListCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "list",
		Short: "list enabled tools",
		Example: `  archery-mcp-server tools list
  archery-mcp-server tools list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig(cmd, *cfgFile, v)
			if err != nil {
				return err
			}
			tools, err := buildToolCatalog(cfg)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printToolSummariesJSON(streams.Out, tools)
			}
			return printToolSummariesTable(streams.Out, tools)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print tools as JSON")
	return command
}

func newToolsDescribeCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:     "describe <tool-name>",
		Aliases: []string{"schema"},
		Short:   "describe an enabled tool schema",
		Example: `  archery-mcp-server tools describe workflows.sql_check
  archery-mcp-server tools describe workflows.sql_check --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig(cmd, *cfgFile, v)
			if err != nil {
				return err
			}
			tools, err := buildToolCatalog(cfg)
			if err != nil {
				return err
			}
			tool, ok := findTool(tools, args[0])
			if !ok {
				return fmt.Errorf("unknown tool %q; run 'archery-mcp-server tools list' to see available tools", args[0])
			}
			description := newToolDescription(tool)
			if jsonOutput {
				enc := json.NewEncoder(streams.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(description)
			}
			return printToolDescription(streams.Out, description)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print tool schema as JSON")
	return command
}

func newToolsCallCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	var rawParams string
	var paramsFile string
	command := &cobra.Command{
		Use:   "call <tool-name>",
		Short: "call an enabled tool with JSON parameters",
		Example: `  archery-mcp-server --config config.yaml tools call workflows.sql_check --params '{"instance_id":1,"db_name":"app","full_sql":"select 1"}'
	  echo '{"auth_type":"totp","otp":123456}' | archery-mcp-server --config config.yaml tools call service_2fa.verify --params-file -`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig(cmd, *cfgFile, v)
			if err != nil {
				return err
			}
			params, err := parseToolParams(rawParams, paramsFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			tools, err := buildToolCatalog(cfg)
			if err != nil {
				return err
			}
			tool, ok := findTool(tools, args[0])
			if !ok {
				return fmt.Errorf("unknown tool %q; run 'archery-mcp-server tools list' to see available tools", args[0])
			}
			if err := validateToolParams(tool, params); err != nil {
				return err
			}
			result, err := tool.Handler(cmd.Context(), params)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(streams.Out, result)
			return err
		},
	}
	command.Flags().StringVar(&rawParams, "params", "{}", "tool parameters as a JSON object")
	command.Flags().StringVar(&paramsFile, "params-file", "", "path to a JSON file containing tool parameters; use - to read stdin")
	return command
}

func validateToolParams(tool toolset.ServerTool, params map[string]any) error {
	schemaJSON := tool.Tool.RawInputSchema
	if len(schemaJSON) == 0 {
		if tool.Tool.InputSchema.Type == "" && len(tool.Tool.InputSchema.Properties) == 0 && len(tool.Tool.InputSchema.Required) == 0 && tool.Tool.InputSchema.AdditionalProperties == nil {
			return nil
		}
		var err error
		schemaJSON, err = json.Marshal(tool.Tool.InputSchema)
		if err != nil {
			return fmt.Errorf("marshal input schema for tool %q: %w", tool.Tool.Name, err)
		}
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("decode input schema for tool %q: %w", tool.Tool.Name, err)
	}
	resourceURL := fmt.Sprintf("mem:///archery-mcp-server/tools/%s/input-schema.json", tool.Tool.Name)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resourceURL, document); err != nil {
		return fmt.Errorf("register input schema for tool %q: %w", tool.Tool.Name, err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		return fmt.Errorf("compile input schema for tool %q: %w", tool.Tool.Name, err)
	}

	encodedParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode parameters for tool %q: %w", tool.Tool.Name, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(encodedParams))
	if err != nil {
		return fmt.Errorf("decode parameters for tool %q: %w", tool.Tool.Name, err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("input schema validation failed for tool %q: %w", tool.Tool.Name, err)
	}
	return nil
}

func findTool(tools []toolset.ServerTool, name string) (toolset.ServerTool, bool) {
	for _, tool := range tools {
		if tool.Tool.Name == name {
			return tool, true
		}
	}
	return toolset.ServerTool{}, false
}

type toolSummary struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	Description string `json:"description,omitempty"`
}

type toolDescription struct {
	Name        string         `json:"name"`
	Domain      string         `json:"domain"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

func newToolDescription(tool toolset.ServerTool) toolDescription {
	desc := toolDescription{
		Name:        tool.Tool.Name,
		Domain:      tool.Domain,
		Description: tool.Tool.Description,
	}
	if tool.Tool.RawInputSchema != nil {
		_ = json.Unmarshal(tool.Tool.RawInputSchema, &desc.InputSchema)
	} else if tool.Tool.InputSchema.Type != "" || len(tool.Tool.InputSchema.Properties) > 0 {
		raw, err := json.Marshal(tool.Tool.InputSchema)
		if err == nil {
			_ = json.Unmarshal(raw, &desc.InputSchema)
		}
	}
	return desc
}

func printToolSummariesTable(out io.Writer, tools []toolset.ServerTool) error {
	sorted := append([]toolset.ServerTool(nil), tools...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Tool.Name < sorted[j].Tool.Name
	})
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tDOMAIN\tDESCRIPTION")
	for _, tool := range sorted {
		desc := strings.ReplaceAll(tool.Tool.Description, "\n", " ")
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", tool.Tool.Name, tool.Domain, desc)
	}
	return w.Flush()
}

func printToolSummariesJSON(out io.Writer, tools []toolset.ServerTool) error {
	summaries := make([]toolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, toolSummary{
			Name:        tool.Tool.Name,
			Domain:      tool.Domain,
			Description: tool.Tool.Description,
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(summaries)
}

func printToolDescription(out io.Writer, description toolDescription) error {
	if _, err := fmt.Fprintf(out, "Name:        %s\n", description.Name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Domain:      %s\n", description.Domain); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Description: %s\n", description.Description); err != nil {
		return err
	}
	if description.InputSchema != nil {
		raw, err := json.MarshalIndent(description.InputSchema, "", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "InputSchema:\n%s\n", raw); err != nil {
			return err
		}
	}
	return nil
}

func parseToolParams(rawParams, paramsFile string, stdin io.Reader) (map[string]any, error) {
	payload := strings.TrimSpace(rawParams)
	if paramsFile != "" {
		var data []byte
		var err error
		if paramsFile == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(paramsFile)
		}
		if err != nil {
			return nil, fmt.Errorf("read params file: %w", err)
		}
		payload = string(data)
	}
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	params := map[string]any{}
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return nil, fmt.Errorf("parse tool params JSON: %w", err)
	}
	if params == nil {
		return nil, fmt.Errorf("parse tool params JSON: expected an object")
	}
	return params, nil
}
