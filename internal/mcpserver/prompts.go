package mcpserver

// MCP prompts: reusable conversation starters that pre-load OpenProject
// context and tell the model what to produce. They never act on their own;
// the model follows up with tool calls. Everything they load is available
// through tools too (clients without prompt support lose convenience, not
// capability).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type promptDef struct {
	name, title, description string
	args                     []*mcp.PromptArgument
	build                    func(ctx context.Context, env *ops.Env, args map[string]string) (string, error)
}

var prompts = []promptDef{
	{
		name: "my_work", title: "My work",
		description: "Load my open work packages and propose a prioritised plan for today",
		args:        []*mcp.PromptArgument{{Name: "project", Description: "Limit to this project (default: all)"}},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			project := orDefault(a["project"], "all")
			wps, err := run(ctx, env, "op_list_work_packages", ops.Args{"assignee": "me", "project": project, "max": 100, "sort": "dueDate:asc"})
			if err != nil {
				return "", err
			}
			return section("My open work packages (sorted by due date)", wps) + `
Today is ` + today() + `. Build my plan for today:
1. Flag anything overdue or due within 3 days.
2. Propose the 3-5 work packages to focus on today, with a one-line reason each.
3. Point out work packages that look stale (not updated for a long time) or blocked.
4. Suggest status updates I should make; do not apply them without my confirmation.
Reference work packages as #id with their subject. Be concise.`, nil
		},
	},
	{
		name: "triage_work_package", title: "Triage work package",
		description: "Load a work package with its history and valid transitions, and assess it",
		args:        []*mcp.PromptArgument{{Name: "id", Description: "Work package id", Required: true}},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			id := a["id"]
			wp, err := run(ctx, env, "op_get_work_package", ops.Args{"id": id, "activities": true})
			if err != nil {
				return "", err
			}
			allowed, err := run(ctx, env, "op_list_work_package_allowed_values", ops.Args{"id": id})
			if err != nil {
				return "", err
			}
			return section("Work package #"+id, wp) + section("Allowed values right now", allowed) + `
Triage this work package:
1. Is the type and priority right? Suggest changes with a one-line rationale.
2. What information is missing to act on it (acceptance criteria, repro steps, scope, estimate, dates)?
3. Suggest the next status (only from the allowed values) and an assignee if one can be inferred.
4. Summarise the discussion so far in 3 bullets if there are comments.
Propose the op_update_work_package / op_comment_work_package calls, but wait for confirmation before calling them.`, nil
		},
	},
	{
		name: "project_status_report", title: "Project status report",
		description: "Summarise a project's state: progress, recent changes, overdue and at-risk work",
		args: []*mcp.PromptArgument{
			{Name: "project", Description: "Project id, identifier or name", Required: true},
			{Name: "days", Description: "Look-back window in days for recent changes (default 7)"},
		},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			days := orDefault(a["days"], "7")
			project, err := run(ctx, env, "op_get_project", ops.Args{"project": a["project"]})
			if err != nil {
				return "", err
			}
			open, err := run(ctx, env, "op_list_work_packages", ops.Args{"project": a["project"], "max": 200, "sort": "dueDate:asc"})
			if err != nil {
				return "", err
			}
			recent, err := run(ctx, env, "op_list_work_packages", ops.Args{
				"project": a["project"], "status": "all", "max": 100,
				"filter": fmt.Sprintf(`[{"updatedAt":{"operator":">t-","values":["%s"]}}]`, days),
			})
			if err != nil {
				return "", err
			}
			return section("Project", project) + section("Open work packages", open) +
				section("Work packages updated in the last "+days+" days (any status)", recent) + `
Today is ` + today() + `. Write a status report for stakeholders:
- Headline: overall health (on track / at risk / off track) with a one-sentence justification.
- Done recently (closed in the window) and in progress.
- Overdue and due within 7 days, with assignees.
- Risks and blockers (unassigned important work, stale items, high priority open bugs).
- Suggested next steps.
Use Markdown, keep it under one page, reference work packages as #id.`, nil
		},
	},
	{
		name: "log_time", title: "Log my time",
		description: "Turn a free-text description of what I did into time entries on the right work packages",
		args: []*mcp.PromptArgument{
			{Name: "summary", Description: "What you worked on, in your own words (e.g. '2h review of login bug, 1h standup')", Required: true},
			{Name: "date", Description: "Day of the work (default today)"},
		},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			date := orDefault(a["date"], "today")
			wps, err := run(ctx, env, "op_list_work_packages", ops.Args{"assignee": "me", "project": "all", "max": 100})
			if err != nil {
				return "", err
			}
			logged, err := run(ctx, env, "op_list_time_entries", ops.Args{"from": date, "to": date})
			if err != nil {
				return "", err
			}
			acts, err := run(ctx, env, "op_list_time_activities", nil)
			if err != nil {
				return "", err
			}
			return "What I did (" + date + "):\n" + a["summary"] + "\n\n" +
				section("My open work packages", wps) + section("Already logged that day", logged) + section("Time activities", acts) + `
Map my description to time entries:
1. For each chunk of work pick the best matching work package (search with op_list_work_packages if none of mine fits) and an activity.
2. Show a table: work package, hours, activity, comment. Flag anything ambiguous and ask.
3. Warn if the day would exceed 10h or duplicates what is already logged.
4. After I confirm, create them with op_log_time (date "` + date + `").`, nil
		},
	},
	{
		name: "timesheet_review", title: "Timesheet review",
		description: "Review my logged time in a period: totals per day/project, gaps and anomalies",
		args: []*mcp.PromptArgument{
			{Name: "from", Description: "First day (default: Monday of this week)"},
			{Name: "to", Description: "Last day (default: today)"},
			{Name: "user", Description: "User to review (default: me)"},
		},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			args := ops.Args{"from": orDefault(a["from"], "week"), "to": orDefault(a["to"], "today"), "user": orDefault(a["user"], "me"), "max": 0}
			entries, err := run(ctx, env, "op_list_time_entries", args)
			if err != nil {
				return "", err
			}
			return section(fmt.Sprintf("Time entries %v → %v", args["from"], args["to"]), entries) + `
Review this timesheet assuming 8h working days Monday-Friday:
- Totals per day and per project (tables).
- Missing or short days, days over 10h, weekend entries.
- Entries without comment or with vague comments.
- Suggested fixes (op_log_time / op_update_time_entry), applied only after my confirmation.`, nil
		},
	},
	{
		name: "draft_work_package", title: "Draft work package",
		description: "Turn a rough idea, bug report or email into a well-formed work package ready to create",
		args: []*mcp.PromptArgument{
			{Name: "text", Description: "Raw text: idea, bug report, email, meeting notes...", Required: true},
			{Name: "project", Description: "Target project (default: configured project)"},
		},
		build: func(ctx context.Context, env *ops.Env, a map[string]string) (string, error) {
			typeArgs := ops.Args{}
			if a["project"] != "" {
				typeArgs["project"] = a["project"]
			}
			types, err := run(ctx, env, "op_list_types", typeArgs)
			if err != nil {
				return "", err
			}
			prios, err := run(ctx, env, "op_list_priorities", nil)
			if err != nil {
				return "", err
			}
			target := orDefault(a["project"], "the configured default project")
			return "Raw input:\n" + a["text"] + "\n\n" + section("Available types", types) + section("Priorities", prios) + `
Draft a work package for ` + target + `:
- subject: imperative, under 80 characters.
- type and priority chosen from the lists above, with a one-line justification.
- description in Markdown: context, (for bugs) steps to reproduce / expected / actual, acceptance criteria as a checklist.
- If it is too big, propose splitting it into a parent and children.
Before creating, search for duplicates with op_list_work_packages (search param). Show the draft and call op_create_work_package only after I confirm.`, nil
		},
	},
}

// PromptNames lists the prompt names and descriptions (for `opcli mcp prompts`).
func PromptNames() [][2]string {
	out := make([][2]string, len(prompts))
	for i, p := range prompts {
		out[i] = [2]string{p.name, p.description}
	}
	return out
}

func registerPrompts(s *mcp.Server, env *ops.Env) {
	for _, p := range prompts {
		s.AddPrompt(&mcp.Prompt{Name: p.name, Title: p.title, Description: p.description, Arguments: p.args},
			func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				args := req.Params.Arguments
				for _, a := range p.args {
					if a.Required && strings.TrimSpace(args[a.Name]) == "" {
						return nil, fmt.Errorf("argument %q is required", a.Name)
					}
				}
				text, err := p.build(ctx, env, args)
				if err != nil {
					return nil, err
				}
				return &mcp.GetPromptResult{
					Description: p.title,
					Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
				}, nil
			})
	}
}

func section(title string, v any) string {
	text, err := toText(v)
	if err != nil {
		text = fmt.Sprint(v)
	}
	return "## " + title + "\n```json\n" + text + "\n```\n\n"
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func today() string { return time.Now().Format("Monday 2006-01-02") }
