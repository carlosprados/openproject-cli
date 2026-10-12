# opcli — OpenProject from the command line

`opcli` puts [OpenProject](https://www.openproject.org/) in your terminal and in
your AI agent. One Go binary:

- a **CLI** for humans, fully self-documented (`--help` with examples on every
  command, help topics);
- an embedded **agent skill** (`opcli skill install`) that teaches coding
  agents such as Claude Code to drive that same CLI correctly: commands, JSON
  shapes, names instead of ids, workflows and pitfalls.

Agents use the shell, not a protocol server: one surface to maintain, and the
skill is tested against the real command tree so it never drifts.

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
  children, attachments, history), create, update, delete, comment and edit
  comments, allowed values (workflow-aware status transitions), relations,
  watchers, attachments, parent/child hierarchies.
- **Markdown with images**: `![shot](shot.png)` in a description or comment is
  uploaded as an attachment and rendered; `--attach` sends files along with a
  create or update (on create they are claimed atomically by the new work
  package).
- **Backfill**: `opcli wp backfill` records work done without planning or
  logging — real dates, hours spread over working days, an audit comment.
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
- **Safe for agents and scripts**: `-o json` on every command, writes included;
  destructive commands fail without a terminal unless `--yes`; lock versions
  and retries handled internally; one-line errors that name the fix.

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
`language: es` (`OPENPROJECT_LANGUAGE`) writes generated comments, such as the
`wp backfill` audit note, in Spanish; the default is English.

## Use it

Everything is discoverable from the binary itself:

```sh
opcli --help              # command map
opcli wp --help           # a group
opcli wp list --help      # flags + examples
opcli skill show          # the agent guide: concepts, recipes, pitfalls
opcli help filters        # raw API filter operators
opcli help output         # JSON shapes
```

Common recipes:

```sh
opcli wp list -p website --type Bug --search "timeout"
opcli wp list --status all --filter '[{"updatedAt":{"operator":">t-","values":["3"]}}]'
opcli wp get 42 --activities
opcli wp allowed 42 --field status                 # valid transitions right now
opcli wp fields -p website --type Bug              # fields, custom ones included, and their values
opcli wp create -p website --subject "Fix login" --type Bug --priority High --assignee me
opcli wp update 42 --field "Customer=ACME" --field "Tags=urgent,customer"
opcli wp list -p website --field "Customer=ACME"
opcli wp update 42 --status Closed --comment "Done, @jane please verify" --dry-run
cat spec.md | opcli wp create --subject "SSO epic" --type Epic --description-file -
opcli wp relation add 43 --type follows --to 42
opcli wp backfill -p website --subject "Hotfix" --assignee jane --from 2026-09-14 --to 2026-09-18 --hours 12h --dry-run  # unplanned work, real dates
opcli time list --from month --group-by project
opcli notification list
opcli api describe POST /api/v3/news && opcli api request POST news --body-file news.json
```

Full reference: [docs/commands.md](docs/commands.md) (generated with `opcli docs`).

## AI agents

```sh
opcli skill install          # ~/.claude/skills/opcli/SKILL.md (Claude Code)
opcli skill install --repo   # ./.claude/skills/opcli/SKILL.md, this repository only
opcli skill show             # print it, for any other agent
```

The skill is embedded in the binary, so it always matches it: re-run
`opcli skill install --force` after upgrading (`task install` does it). It
covers every command, the JSON shapes, Markdown, images and mentions, custom
fields by name, `--dry-run` and the server-side validation of every write,
the destructive `--yes` rule, what opcli cannot do, the errors and recipes for common requests
(my day, triage, status report, logging time from free text, timesheet review,
turning notes into work packages).

The MCP server of v0.3 and earlier is gone: stay on v0.3.0 if you need it.

## Architecture

```text
cmd/opcli/   main package
internal/
  ops/        command registry: one Op = params + help + safety + handler
  cli/        builds the Cobra tree from ops (+ config, skill, docs, help topics)
  skill/      SKILL.md, embedded in the binary
  client/     HTTP client: API key auth, retries, error decoding, paging, uploads
  hal/        HAL+JSON flattening, ISO 8601 durations
  spec/       embedded OpenAPI spec index for `opcli api`
  config/     Viper-based configuration
```

Adding a capability means adding one `Op` in `internal/ops` and documenting it
in `SKILL.md` in the same change; `go test ./internal/cli/` fails otherwise.
See [CLAUDE.md](CLAUDE.md) for the development rules.

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
