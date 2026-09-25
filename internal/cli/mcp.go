package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"text/tabwriter"

	"github.com/carlosprados/openproject-cli/internal/mcpserver"
	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func mcpCmd(st *state) *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Run opcli as an MCP server for AI agents, and inspect what it exposes",
		Long:  "Model Context Protocol server. See 'opcli help agents' for an overview.",
	}

	var readOnly bool
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server on stdio",
		Long: `Start an MCP server on stdin/stdout. Logs go to stderr. Credentials come
from the usual config (file or OPENPROJECT_* env), checked at startup.

--read-only hides every tool that modifies data; op_api_request stays
available but only for GET.`,
		Example: `  opcli mcp serve
  opcli mcp serve --read-only
  claude mcp add openproject -- opcli mcp serve`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := st.env()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			s := mcpserver.New(env, mcpserver.Options{Version: st.build.Version, ReadOnly: readOnly})
			return s.Run(ctx, &mcp.StdioTransport{})
		},
	}
	serve.Flags().BoolVar(&readOnly, "read-only", false, "Expose only tools that do not modify data")

	tools := &cobra.Command{
		Use:     "tools",
		Short:   "List the MCP tools and their CLI equivalents",
		Example: "  opcli mcp tools\n  opcli mcp tools --read-only\n  opcli mcp tools -o json   # full tool definitions (schemas)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list := mcpserver.Tools(readOnly)
			if st.output() == "json" {
				defs := make([]*mcp.Tool, len(list))
				for i, op := range list {
					defs[i] = mcpserver.Tool(op)
				}
				return ops.RenderJSON(cmd.OutOrStdout(), defs)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TOOL\tCLI\tEFFECT\tSUMMARY")
			for _, op := range list {
				fmt.Fprintf(tw, "%s\topcli %s\t%s\t%s\n", op.Tool, strings.Join(op.Cmd, " "), strings.Trim(safetyLabel(op.Safety), "*"), op.Short)
			}
			return tw.Flush()
		},
	}
	tools.Flags().BoolVar(&readOnly, "read-only", false, "Only tools available in read-only mode")

	resources := &cobra.Command{
		Use:   "resources",
		Short: "List the MCP resources, resource templates and prompts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "RESOURCE\tDESCRIPTION")
			for _, r := range mcpserver.Resources() {
				fmt.Fprintf(tw, "%s\t%s\n", r.URI, r.Description)
			}
			fmt.Fprintln(tw, "\nPROMPT\tDESCRIPTION")
			for _, p := range mcpserver.PromptNames() {
				fmt.Fprintf(tw, "%s\t%s\n", p[0], p[1])
			}
			return tw.Flush()
		},
	}

	config := &cobra.Command{
		Use:   "config",
		Short: "Print the snippet to register opcli in an MCP client",
		Long: `Print a JSON snippet for MCP clients that read "mcpServers" (Claude
Desktop, Cursor, Gemini CLI, .mcp.json in Claude Code...). The binary path
is resolved from PATH. Credentials are not included: they are read from
~/.openproject.yaml or OPENPROJECT_* env at runtime.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bin, err := exec.LookPath("opcli")
			if err != nil {
				if bin, err = os.Executable(); err != nil {
					bin = "opcli"
				}
			}
			args := []string{"mcp", "serve"}
			if readOnly {
				args = append(args, "--read-only")
			}
			snippet := map[string]any{"mcpServers": map[string]any{"openproject": map[string]any{"command": bin, "args": args}}}
			b, _ := json.MarshalIndent(snippet, "", "  ")
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n\nClaude Code:\n  claude mcp add openproject -- %s %s\n", b, bin, strings.Join(args, " "))
			return nil
		},
	}
	config.Flags().BoolVar(&readOnly, "read-only", false, "Register the server in read-only mode")

	c.AddCommand(serve, tools, resources, config)
	return c
}
