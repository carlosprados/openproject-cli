# opcli — OpenProject from the terminal and from AI agents

opcli wraps the OpenProject API v3. Every capability exists twice with the
same parameters: as a CLI command (`opcli wp list --assignee me`) and as an
MCP tool (`op_list_work_packages {"assignee":"me"}`). Flags are kebab-case,
tool parameters are snake_case: `--due-date` ↔ `due_date`.

## Mental model

- **Project**: container. Reference it by id, identifier (`demo-project`) or name.
- **Work package (WP)**: a task, bug, feature, epic, milestone... Has type,
  status, priority, assignee, accountable (`responsible`), dates, estimate,
  parent/children, relations, watchers, attachments, comments (activities).
- **Time entry**: hours logged on a work package on a given day, with an activity.
- **Version**: release/sprint a WP targets. **Membership**: user + roles in a project.

## Names instead of ids

All references are resolved for you:

| Field          | Accepted values                                              |
|----------------|--------------------------------------------------------------|
| project        | id, identifier, (partial) name; `all` in list commands       |
| status         | name or id; in `wp list` also `open`, `closed`, `all`        |
| type/priority  | name or id (`Bug`, `High`)                                   |
| users          | `me`, `none` (unassign), login, email, (partial) name, id    |
| version        | name (needs the project) or id; `none` clears                |
| dates          | `YYYY-MM-DD`, `today`, `yesterday`, `tomorrow`, `week`, `month`, `+7d`, `-3d`; `none` clears |
| durations      | `1.5`, `1,5`, `90m`, `1h30m`, `PT1H30M`                      |

Unknown or ambiguous names fail with the list of valid values, so a failed
call tells you how to fix it.

## Everyday recipes

```sh
opcli whoami                                        # check connection
opcli project list                                  # what exists
opcli wp list --assignee me --project all           # my open work
opcli wp list -p demo-project --type Bug --search login
opcli wp get 42 --activities                        # full detail + history
opcli wp allowed 42 --field status                  # valid transitions now
opcli wp update 42 --status "In progress" --assignee me
opcli wp update 42 --status Closed --comment "Fixed in 1.2"
opcli wp create -p demo-project --subject "Fix login" --type Bug --priority High
opcli wp comment 42 --body "Deployed to staging"
opcli wp relation add 43 --type follows --to 42
opcli time log --wp 42 --hours 1h30m --comment "Code review" --activity Development
opcli time list --from week --group-by day          # my week
opcli notification list                             # unread inbox
```

## Rules of thumb (for agents especially)

1. Start with `op_whoami` if unsure the server is reachable.
2. Status changes are constrained by workflows: call
   `op_list_work_package_allowed_values` (field `status`) before guessing.
3. Updates only touch the parameters you send. Lock versions and conflicts
   are handled internally.
4. Descriptions and comments are Markdown.
5. Lists return `{total, count, items}`; `total > count` means there is more
   (raise `max` or use `page`).
6. Destructive tools (`op_delete_*`) cannot be undone: confirm with the
   user first.
7. Anything not covered by a dedicated tool: find it with
   `op_api_list_endpoints`, read it with `op_api_describe_endpoint`, call
   it with `op_api_request`.

## Output

CLI: `-o text` (default, tables) or `-o json` (the exact payload MCP tools
return). Records are flattened from HAL: a link `status` becomes
`"status": "In progress"` plus `"statusId": 7`; Markdown fields are plain
strings; durations stay ISO 8601 (`PT1H30M`).

## Configuration

`opcli config init --url https://op.example.com --api-key <key>` writes
`~/.openproject.yaml` (mode 0600). Environment variables override it:
`OPENPROJECT_URL`, `OPENPROJECT_API_KEY`, `OPENPROJECT_PROJECT` (default
project). Create the API key in OpenProject → My account → Access tokens → API.

## MCP

`opcli mcp serve` speaks MCP over stdio. `--read-only` exposes only tools
that do not modify data (plus GET-only `op_api_request`). Besides tools the
server offers resources (`openproject://...`) with ready-made context and
prompts for common workflows (my work, triage, status report, time logging,
timesheet review, drafting work packages).
