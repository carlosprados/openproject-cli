// Package mcpserver exposes opcli over the Model Context Protocol.
//
// Tools are generated from internal/ops, so each tool is the exact twin of a
// CLI command (same handler, same params in snake_case). Resources and
// prompts are sugar on top of those same ops: nothing here reaches the API
// in a way a tool cannot (see CLAUDE.md, "parity rules").
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/guide"
	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configure the server.
type Options struct {
	Version  string
	ReadOnly bool
}

// New builds an MCP server bound to env.
func New(env *ops.Env, opt Options) *mcp.Server {
	env.ReadOnly = opt.ReadOnly
	s := mcp.NewServer(
		&mcp.Implementation{Name: "openproject", Title: "OpenProject (opcli)", Version: opt.Version},
		&mcp.ServerOptions{Instructions: guide.Instructions},
	)
	for _, op := range Tools(opt.ReadOnly) {
		s.AddTool(Tool(op), handler(env, op))
	}
	registerResources(s, env)
	registerPrompts(s, env)
	return s
}

// Tools returns the ops exposed as tools in the given mode.
func Tools(readOnly bool) []*ops.Op {
	var out []*ops.Op
	for _, op := range ops.All() {
		if readOnly && op.Safety != ops.Read && !op.ReadOnlySafe {
			continue
		}
		out = append(out, op)
	}
	return out
}

var cliFlag = regexp.MustCompile(`--([a-z][a-z-]*[a-z])`)

// mcpText rewrites CLI flag mentions (--due-date) into parameter names
// (due_date) so tool descriptions speak the agent's vocabulary.
func mcpText(s string) string {
	return cliFlag.ReplaceAllStringFunc(s, func(f string) string {
		return strings.ReplaceAll(strings.TrimPrefix(f, "--"), "-", "_")
	})
}

// Tool builds the MCP tool definition of op.
func Tool(op *ops.Op) *mcp.Tool {
	desc := mcpText(op.Short)
	if op.Long != "" {
		desc += "\n\n" + mcpText(op.Long)
	}
	desc += fmt.Sprintf("\n\nCLI equivalent: opcli %s", strings.Join(op.Cmd, " "))

	props := map[string]any{}
	var required []string
	for _, p := range op.Params {
		prop := map[string]any{"description": mcpText(p.Desc)}
		switch p.Kind {
		case ops.String:
			prop["type"] = "string"
		case ops.Int:
			prop["type"] = "integer"
		case ops.Bool:
			prop["type"] = "boolean"
		case ops.Strings:
			prop["type"] = "array"
			prop["items"] = map[string]any{"type": "string"}
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		props[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}

	destructive := op.Safety == ops.Destructive
	openWorld := false
	return &mcp.Tool{
		Name:        op.Tool,
		Title:       mcpText(op.Short),
		Description: desc,
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title:           mcpText(op.Short),
			ReadOnlyHint:    op.Safety == ops.Read,
			DestructiveHint: &destructive,
			IdempotentHint:  op.Safety == ops.Read,
			OpenWorldHint:   &openWorld,
		},
	}
}

func handler(env *ops.Env, op *ops.Op) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := ops.Args{}
		if raw := req.Params.Arguments; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return errorResult(fmt.Errorf("invalid arguments: %w", err)), nil
			}
		}
		res, err := op.Execute(ctx, env, args)
		if err != nil {
			return errorResult(err), nil
		}
		text, err := toText(res)
		if err != nil {
			return errorResult(err), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	}
}

// Errors are tool-level results (IsError) so the model can read and react.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}}}
}

func toText(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// run executes an op by tool name; used by resources and prompts.
func run(ctx context.Context, env *ops.Env, tool string, args ops.Args) (any, error) {
	op := ops.ByTool(tool)
	if op == nil {
		return nil, fmt.Errorf("internal: unknown op %s", tool)
	}
	if args == nil {
		args = ops.Args{}
	}
	return op.Execute(ctx, env, args)
}
