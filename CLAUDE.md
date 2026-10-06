# CLAUDE.md

Guidance for Claude Code (and humans) working on this repository.

## Build and run

```bash
go build -o opcli ./cmd/opcli   # build
go vet ./...            # lint
go test ./...           # tests (no server needed: httptest + in-memory MCP)
go run ./cmd/opcli docs > docs/commands.md   # regenerate the reference (CI diffs it)
task install                    # build with version/commit/date into ~/Dropbox/Charlie/bin
task release V=vX.Y.Z           # vet + test, tag, push tag (goreleaser), install
```

`task release` refuses unless you are on a clean `main` with a fresh
`docs/commands.md`. The tag push publishes the GitHub release, so it needs
explicit approval like any push. Every release ends with the binary installed
locally; check it with `opcli --version`.

Running against a server needs `~/.openproject.yaml` or `OPENPROJECT_URL` +
`OPENPROJECT_API_KEY`. Never commit credentials, `.env`, or server URLs/IPs.

## Architecture

Target: **OpenProject API v3** (HAL+JSON), tested on 17.8. Two surfaces, one
source of truth:

- `internal/ops` — the registry. Each `Op` declares `Tool` (MCP name), `Cmd`
  (CLI path), `Params`, `Short/Long/Example`, `Safety` and `Run`. Groups
  (`registerGroup`) are CLI parents; nested groups use spaces ("wp relation").
- `internal/cli` — generates Cobra commands from ops: params → flags
  (snake_case → kebab-case), `File` params get `--x-file` (path or `-`),
  enums get shell completion, output via `-o text|json`.
- `internal/mcpserver` — generates MCP tools from the same ops (params → JSON
  schema, `Safety` → annotations, CLI flag mentions in help rewritten to param
  names). Resources and prompts call ops through `run()`.
- `internal/hal` — `Flatten` is the single place that turns HAL into flat
  records; list handlers compact further (see `wpListNoise`).
- `internal/client` — all HTTP goes through `Raw()`: auth (`apikey:<key>`),
  429/503 retry, `APIError` decoding, `Collect()` paging.
- `internal/spec` — embedded, sanitised OpenAPI spec (`openapi.json.gz`).

### OpenProject specifics worth remembering

- Writes use `_links: {x: {href}}`; `{"href": null}` unsets a link.
- Work package PATCH needs `lockVersion`; `op_update_work_package` refetches
  and retries once on 409.
- In 17.x the version link is `targetVersions` (array) and the list filter is
  `targetVersion`. Time entries link to `entity` (work package or meeting);
  filters are `entity_type`/`entity_id`, `spent_on` (`<>d`), `user`, `project_id`.
- Time-entry activities are only listed through `POST /time_entries/form`.
- Status transitions depend on workflows: `/work_packages/{id}/form` schema
  `allowedValues` is the truth (`op_list_work_package_allowed_values`).
- `notify=false` is a query parameter, not a body field.
- Activity `user` links carry no title; `nameUser` resolves names.

## Development rules

### CLI ↔ MCP parity is structural

Never add a Cobra command or an MCP tool by hand for server functionality:
add an `Op`. `TestRegistryConventions` and `TestToolsMirrorOps` enforce unique
names, snake_case params, no flag collisions (including globals `-o`,
`--output`, `--url`, `--api-key`, `--config`), mandatory `Short` + `Example`,
and MCP descriptions free of CLI flag syntax.

Local-only commands (`config`, `mcp`, `guide`, `docs`, help topics) live in
`internal/cli` because they do not touch the server.

### Resources and prompts are sugar

No functionality may exist only as a resource or prompt. They must be built
from ops that are also tools (`run(ctx, env, "op_...", args)`), so tool-only
clients (e.g. LM Studio) can do everything.

### Help text is product

`Short`, `Long` and `Example` are what humans read in `--help` and what agents
read in tool descriptions. Write them for both: concrete, with real flag
names, accepted value formats and one example per common use.

### Other

- Comments only where the code is not obvious; English everywhere in code.
- Refreshing the embedded spec: `task spec` (it sanitises `servers[].url`;
  `TestEmbeddedSpecIsSanitised` guards it) and bump `spec.EmbeddedVersion`.

## Git conventions

- Branches: `opcli/<feature>`. Conventional prefixes: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`.
- **No `Co-Authored-By: Claude` trailers** in commit messages.
