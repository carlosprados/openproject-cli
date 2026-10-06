# opcli — OpenProject CLI and MCP server

`opcli` puts [OpenProject](https://www.openproject.org/) in your terminal and in
your AI agent. One Go binary, two faces:

- a **CLI** for humans, fully self-documented (`--help` with examples on every
  command, help topics, a built-in guide);
- an **MCP server** for AI agents (Claude Code, Claude Desktop, Cursor, Gemini
  CLI...) with **tools, resources and prompts**.

Both faces are generated from the same command registry, so they can never
drift apart: every CLI command *is* an MCP tool with the same parameters.

```text
$ opcli wp list --assignee me --project all
ID  TYPE  STATUS       SUBJECT                     ASSIGNEE      PRIORITY  PROJECT       DUE
42  Bug   In progress  Login times out after 30s   Jane Doe      High      Website       2026-10-02
17  Task  New          Write release notes         Jane Doe      Normal    Website       2026-10-09

$ opcli wp update 42 --status Closed --comment "Fixed in 1.4.2"
$ opcli time log --wp 42 --hours 1h30m --activity Development --comment "Root cause + fix"
```

Tested against OpenProject 17.8 (API v3).

## Features

- **Work packages**: list/search with filters, view (description, relations,
  children, attachments, history), create, update, delete, comment, allowed
  values (workflow-aware status transitions), relations, watchers, attachments.
- **Time tracking**: log, list with totals and grouping (day/project/wp/activity/user), update, delete.
- **Projects, versions, memberships, users, notifications, saved queries, meetings**,
  plus reference data (statuses, types, priorities, roles).
- **Names instead of ids** everywhere: `--project website`, `--status "In progress"`,
  `--assignee me`, `--type Bug`, `--due-date +7d`, `--hours 1h30m`. Ambiguous or
  unknown names fail with the list of valid values.
- **The whole API, not just the curated part**: `opcli api endpoints|describe|request`
  searches an embedded OpenAPI catalogue of ~300 operations and calls any of them.
- **Agent-friendly output**: `-o json` gives flat records (HAL links collapsed to
  `"status": "In progress", "statusId": 7`), lean listings, `{total,count,items}` envelopes.
- **Safe for agents**: MCP tool annotations (read-only / destructive), `--read-only`
  server mode, lock versions and retries handled internally.

## Install

```sh
go install github.com/carlosprados/openproject-cli/cmd/opcli@latest
# or grab a release binary from GitHub Releases
# or from source:
git clone https://github.com/carlosprados/openproject-cli && cd openproject-cli && task install
```

## Configure

Create an API token in OpenProject (*My account → Access tokens → API*), then:

```sh
opcli config init --url https://op.example.com --api-key <token> [--project <default>]
opcli whoami
```

This verifies the token and writes `~/.openproject.yaml` with mode `0600`.
Precedence: flags (`--url`, `--api-key`) > environment (`OPENPROJECT_URL`,
`OPENPROJECT_API_KEY`, `OPENPROJECT_PROJECT`, `OPENPROJECT_DEFAULT_TYPE`) > `.env` in
the current directory > config file. See `opcli help auth`.

New work packages without `--type` get `default_type` from the config
(`--default-type` in `config init`), else the type the project marks as default.

## Use it

Everything is discoverable from the binary itself:

```sh
opcli --help              # command map
opcli wp --help           # a group
opcli wp list --help      # flags + examples
opcli guide               # concepts, conventions, recipes
opcli help filters        # raw API filter operators
opcli help output         # JSON shapes
```

Common recipes:

```sh
opcli wp list -p website --type Bug --search "timeout"
opcli wp list --status all --filter '[{"updatedAt":{"operator":">t-","values":["3"]}}]'
opcli wp get 42 --activities
opcli wp allowed 42 --field status                 # valid transitions right now
opcli wp create -p website --subject "Fix login" --type Bug --priority High --assignee me
cat spec.md | opcli wp create --subject "SSO epic" --type Epic --description-file -
opcli wp relation add 43 --type follows --to 42
opcli wp backfill -p website --subject "Hotfix" --assignee jane --from 2026-09-14 --to 2026-09-18 --hours 12h --dry-run  # unplanned work, real dates
opcli time list --from month --group-by project
opcli notification list
opcli api describe POST /api/v3/news && opcli api request POST news --body-file news.json
```

Full reference: [docs/commands.md](docs/commands.md) (generated with `opcli docs`).

## MCP server

```sh
claude mcp add openproject -- opcli mcp serve          # Claude Code
opcli mcp config                                       # JSON snippet for other clients
opcli mcp serve --read-only                            # no tool that modifies data
```

What the server exposes (`opcli mcp tools`, `opcli mcp resources`):

| Primitive | What |
|---|---|
| **Tools** (52) | One per CLI command: `op_list_work_packages`, `op_update_work_package`, `op_log_time`, `op_api_request`... Flags map to snake_case params (`--due-date` → `due_date`). Annotated read-only / destructive. |
| **Resources** | `openproject://guide`, `me`, `catalog` (statuses, types, priorities, roles, activities), `projects`, `my/work_packages`, `my/time_entries`, `my/notifications`, `api/endpoints`; templates `projects/{project}`, `projects/{project}/work_packages`, `work_packages/{id}`, `work_packages/{id}/activities`. |
| **Prompts** | `my_work`, `triage_work_package`, `project_status_report`, `log_time` (free text → time entries), `timesheet_review`, `draft_work_package`. |

Resources and prompts are conveniences over the same operations the tools
expose, so clients that only support tools lose nothing.

## Architecture

```text
cmd/opcli/   main package
internal/
  ops/        command registry: one Op = params + help + safety + handler
  cli/        builds the Cobra tree from ops (+ config, mcp, guide, docs, help topics)
  mcpserver/  builds MCP tools from ops (+ resources, prompts) — official go-sdk
  client/     HTTP client: API key auth, retries, error decoding, paging, uploads
  hal/        HAL+JSON flattening, ISO 8601 durations
  spec/       embedded OpenAPI spec index for `opcli api`
  config/     Viper-based configuration
  guide/      usage guide shared by `opcli guide` and MCP
```

Adding a capability means adding one `Op` in `internal/ops`; the CLI command,
its help, the MCP tool and its JSON schema all follow. See [CLAUDE.md](CLAUDE.md)
for the development rules.

## Development

```sh
task build   # or: go build -o opcli ./cmd/opcli
task test    # go test ./...
task lint    # gofmt + go vet
task docs    # regenerate docs/commands.md (CI checks it is current)
task spec    # refresh the embedded OpenAPI spec from your server
```

Releases: push a `vX.Y.Z` tag; GoReleaser builds binaries for Linux, macOS and
Windows (amd64/arm64).

## License

[MIT](LICENSE) © Carlos Javier Prados Hijón
