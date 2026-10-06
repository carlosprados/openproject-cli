package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/spf13/cobra"
)

// Help topics: commands without Run show up under "Additional help topics"
// and are read with `opcli help <topic>`.
var helpTopics = []*cobra.Command{
	{
		Use:   "auth",
		Short: "How to authenticate: API key, config file, environment",
		Long: `opcli authenticates with an OpenProject API key.

1. In OpenProject: avatar → My account → Access tokens → API → "+ API token".
   Copy the token (it is shown only once).
2. Save it:
     opcli config init --url https://op.example.com --api-key <token>
   This verifies the token and writes ~/.openproject.yaml with mode 0600.

Alternatives (highest precedence first):
  --url / --api-key flags
  OPENPROJECT_URL, OPENPROJECT_API_KEY, OPENPROJECT_PROJECT,
  OPENPROJECT_DEFAULT_TYPE env vars
  .env file in the current directory (same variable names)
  --config <file> or ~/.openproject.yaml:
      url: https://op.example.com
      api_key: <token>
      project: demo-project   # optional default project
      default_type: Tarea     # optional; else the project's default type

Check with 'opcli whoami' and 'opcli config show'. Username/password is not
supported by the API v3 by design.`,
	},
	{
		Use:   "filters",
		Short: "Raw API filters for --filter (operators and examples)",
		Long: `Most filtering is done with dedicated flags (--status, --assignee...).
For anything else, list commands accept --filter with raw API v3 filters: a
JSON array of {"<filter>": {"operator": "<op>", "values": [...]}}. They are
ANDed with the flags.

Operators:
  =  !         equals / not equals (values: ids)
  o  c         open / closed (status only, values: [])
  *  !*        any value / none
  ~  !~        contains / does not contain (text)
  **           full-text search
  >=  <=       greater/lower or equal
  <>d          between dates [from, to]
  >t-  <t-     more / less than N days ago     (values: ["7"])
  t-  t  w     N days ago / today / this week
  <t+  >t+     in less / more than N days       (values: ["7"])

Work package filter names include: status, type, priority, assignee, author,
responsible, project, parent, targetVersion, subject, description, search,
startDate, dueDate, createdAt, updatedAt, estimatedTime, percentageDone,
watcher, category, id, blocks, blocked, relates, follows, precedes.

Examples:
  # due in the next 7 days
  opcli wp list --filter '[{"dueDate":{"operator":"<t+","values":["7"]}}]'
  # updated in the last 3 days, any status
  opcli wp list --status all --filter '[{"updatedAt":{"operator":">t-","values":["3"]}}]'
  # no due date
  opcli wp list --filter '[{"dueDate":{"operator":"!*","values":[]}}]'

Look up any endpoint's filters with 'opcli api describe GET <path>'.`,
	},
	{
		Use:   "output",
		Short: "Output formats and the shape of JSON results",
		Long: `-o text (default): tables for lists, "key: value" records for single items.
-o json: exactly what the equivalent MCP tool returns.

JSON shapes:
  lists    {"total": 37, "count": 20, "items": [...]}   total > count → more pages
  records  flat objects flattened from HAL:
             "status": "In progress", "statusId": 7
             "description": "<markdown>"
             "estimatedTime": "PT2H"   (ISO 8601 durations)
             "url": "<browser link>"   (work packages)
  actions  {"ok": true, "message": "..."}

Pipe JSON to jq:
  opcli wp list --assignee me -o json | jq -r '.items[] | "\(.id) \(.subject)"'

'opcli api request' prints the raw HAL answer unless --flatten is given.`,
	},
	{
		Use:   "agents",
		Short: "Using opcli from AI agents through MCP",
		Long: `'opcli mcp serve' runs a Model Context Protocol server over stdio.

Tools       every CLI command, same parameters in snake_case
            (--due-date → due_date). 'opcli mcp tools' lists them.
Resources   openproject://guide, openproject://me, openproject://catalog,
            openproject://projects, openproject://my/work_packages, ...
            'opcli mcp resources' lists them.
Prompts     my_work, triage_work_package, project_status_report, log_time,
            timesheet_review, draft_work_package.

--read-only exposes only non-modifying tools (op_api_request is GET-only).

Register it in Claude Code:
  claude mcp add openproject -- opcli mcp serve
or print a JSON snippet for other clients with 'opcli mcp config'.
Credentials come from the usual config (file or OPENPROJECT_* env).`,
	},
}

func docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "docs",
		Short:   "Print a Markdown reference of every command and its MCP tool",
		Example: `  opcli docs > docs/commands.md`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeDocs(cmd.OutOrStdout())
		},
	}
}

func writeDocs(w io.Writer) error {
	fmt.Fprintln(w, "# opcli command reference")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Generated with `opcli docs`. Every command is also an MCP tool with the same parameters (flags in kebab-case ↔ tool parameters in snake_case). Global flags: `--config`, `--url`, `--api-key`, `-o text|json`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Command | MCP tool | Effect | Summary |")
	fmt.Fprintln(w, "|---|---|---|---|")
	all := ops.All()
	for _, op := range all {
		fmt.Fprintf(w, "| `opcli %s` | `%s` | %s | %s |\n", strings.Join(op.Cmd, " "), op.Tool, safetyLabel(op.Safety), op.Short)
	}
	for _, op := range all {
		fmt.Fprintf(w, "\n## opcli %s\n\n%s\n\n", strings.Join(op.Cmd, " "), op.Short)
		if op.Long != "" {
			fmt.Fprintf(w, "%s\n\n", op.Long)
		}
		fmt.Fprintf(w, "MCP tool: `%s` · %s\n\n", op.Tool, safetyLabel(op.Safety))
		if len(op.Params) > 0 {
			fmt.Fprintln(w, "| Flag | MCP param | Type | Default | Description |")
			fmt.Fprintln(w, "|---|---|---|---|---|")
			params := append([]ops.Param(nil), op.Params...)
			sort.SliceStable(params, func(i, j int) bool { return params[i].Positional && !params[j].Positional })
			for _, p := range params {
				flag := "`--" + p.Flag() + "`"
				if p.Positional {
					flag = "`<" + p.Flag() + ">` (arg)"
				}
				def := ""
				if p.Default != nil {
					def = fmt.Sprintf("`%v`", p.Default)
				}
				desc := strings.ReplaceAll(p.Desc, "|", "\\|")
				if p.Required {
					desc = "**required** " + desc
				}
				fmt.Fprintf(w, "| %s | `%s` | %s | %s | %s |\n", flag, p.Name, kindName(p.Kind), def, desc)
			}
			fmt.Fprintln(w)
		}
		if op.Example != "" {
			fmt.Fprintf(w, "```sh\n%s\n```\n", strings.TrimRight(op.Example, "\n"))
		}
	}
	return nil
}

func safetyLabel(s ops.Safety) string {
	switch s {
	case ops.Write:
		return "writes"
	case ops.Destructive:
		return "**destructive**"
	}
	return "read-only"
}

func kindName(k ops.Kind) string {
	return [...]string{"string", "int", "bool", "list"}[k]
}
