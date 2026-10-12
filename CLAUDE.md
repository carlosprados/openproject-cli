# CLAUDE.md

Guidance for Claude Code (and humans) working on this repository.

## Build and run

```bash
go build -o opcli ./cmd/opcli   # build
go vet ./...            # lint
go test ./...           # tests (no server needed: httptest)
go run ./cmd/opcli docs > docs/commands.md   # regenerate the reference (CI diffs it)
task install                    # build into ~/Dropbox/Charlie/bin + opcli skill install --force
task release V=vX.Y.Z           # vet + test, tag, push tag (goreleaser), install
task lab:up                     # throwaway OpenProject 17 in Docker on :8092 (LAB_PORT=…)
task lab:down                   # remove it (no volume: its data goes too)
```

The lab writes an isolated config to `.lab/opcli.yaml` (gitignored): run
`opcli --config .lab/opcli.yaml …`. It has the demo data (`demo-project`,
types Task/Milestone/Summary task, user `admin`) and is the place for live
tests when no real test instance is reachable.

`task release` refuses unless you are on a clean `main` with a fresh
`docs/commands.md`. The tag push publishes the GitHub release, so it needs
explicit approval like any push. Every release ends with the binary installed
locally; check it with `opcli --version`.

Running against a server needs `~/.openproject.yaml` or `OPENPROJECT_URL` +
`OPENPROJECT_API_KEY`. Never commit credentials, `.env`, or server URLs/IPs.

## Architecture

Target: **OpenProject API v3** (HAL+JSON), tested on 17.8. It is a CLI only,
by design: agents drive it from the shell, guided by the embedded skill
(`internal/skill/SKILL.md`). The MCP server was dropped in v0.4.0.

- `internal/ops` — the registry. Each `Op` declares `Cmd` (CLI path),
  `Params`, `Short/Long/Example`, `Safety` and `Run`. Groups
  (`registerGroup`) are CLI parents; nested groups use spaces ("wp relation").
  `attach.go`: `--attach` and local Markdown images (upload, dedupe by name
  and size, rewrite to `/api/v3/attachments/{id}/content`; on create they are
  uploaded containerless and claimed by the POST). `fields.go`: the work
  package form schema → `fieldDef`; `--field NAME=VALUE` conversion by type,
  `wp fields`, `customFields` by name on reads, `--field` filters, and
  `validate` (every create/update goes through the form before any upload or
  write). `mentions.go`: `@login` → `<mention>` tags.
- `internal/cli` — generates Cobra commands from ops: params → flags
  (snake_case → kebab-case), `File` params get `--x-file` (path or `-`; the
  file's directory goes to `ops.FileDirKey` for relative images), enums get
  shell completion, output via `-o text|json`. `Destructive` ops get `--yes`
  and a prompt (`Op.NeedsConfirm`); without a TTY they fail unless `--yes`.
  Write and destructive ops get `--dry-run`: `client.Recorder` records every
  request except reads, `…/form` and `/render/` (so validation still runs),
  and the CLI prints the recorded requests; `wp backfill` checks `Env.DryRun`
  and returns its own plan.
  Usage is printed only for flag/argument errors.
- `internal/skill` + `internal/cli/skill.go` — `opcli skill install|show|path`;
  `SKILL.md` is embedded with `go:embed`, so the installed skill matches the
  binary.
- `internal/hal` — `Flatten` is the single place that turns HAL into flat
  records; list handlers compact further (see `wpListNoise`).
- `internal/client` — all HTTP goes through `Raw()`: auth (`apikey:<key>`),
  429/503 retry, `APIError` decoding, `Collect()` paging.
- `internal/spec` — embedded, sanitised OpenAPI spec (`openapi.json.gz`).

### OpenProject specifics worth remembering

- Writes use `_links: {x: {href}}`; `{"href": null}` unsets a link.
- Work package PATCH needs `lockVersion`; `wp update` refetches
  and retries once on 409.
- In 17.x the version link is `targetVersions` (array) and the list filter is
  `targetVersion`. Time entries link to `entity` (work package or meeting);
  filters are `entity_type`/`entity_id`, `spent_on` (`<>d`), `user`, `project_id`.
- Time-entry activities are only listed through `POST /time_entries/form`.
- Status transitions depend on workflows: `/work_packages/{id}/form` schema
  `allowedValues` is the truth (`wp allowed`).
- `notify=false` is a query parameter, not a body field.
- `PATCH /activities/{id}` takes `comment` as a plain string, although the
  spec says object; `POST /work_packages/{id}/activities` takes `{raw}`.
- Activity `user` links carry no title; `nameUser` resolves names.
- `POST /projects/{id}/work_packages/form` and `/work_packages/{id}/form`
  validate without saving (workflow transitions included) and return the
  schema with `allowedValues` (embedded, an array of links, or an href for
  users). `GET /work_packages/schemas/{project}-{type}` has names but no
  allowed values.
- Custom fields: link-type values go in `_links` (`location: "_links"`),
  the rest in the body; a Link custom field is a plain string. User and
  version custom fields cannot be filtered ("Cf N filter does not exist").
- Mentions are stored as `<mention class="mention" data-id="5"
  data-type="user" data-text="@Name">@Name</mention>`; a plain `@login`
  notifies nobody. Only admins see `login`/`email` of other users, and
  `/principals` has no login filter for others.
- Internal comments (`"internal": true` on POST activities) need an
  Enterprise plan and the project setting.

## Development rules

### Doctrine: the CLI and the skill are twins

`internal/skill/SKILL.md` is what agents read instead of `--help`. A command,
flag or behaviour the skill does not mention does not exist for them, and a
command it shows that no longer runs misleads them. So:

- **Every change to the CLI updates `SKILL.md` in the same commit**: new or
  renamed commands and flags, JSON shapes, formats, errors, limitations. No
  exceptions, no "docs later".
- `go test ./internal/cli/` enforces it (`skill_doctrine_test.go`): every
  command and flag must appear in the skill, and every `opcli …` line in its
  `sh` blocks must parse against the real command tree. A deliberate omission
  goes in `notForAgents` with its reason; keep that list short.
- The test cannot judge meaning: when behaviour changes, reread the affected
  skill section and fix it by hand, then run the commands for real against
  the test project.
- Keep it short and generic (no instance URLs, project names or people): it
  is public and installed on other people's machines.

### Commands come from ops

Never add a Cobra command by hand for server functionality: add an `Op`.
`TestRegistryConventions` enforces unique paths, snake_case params, no flag
collisions (including globals `-o`, `--output`, `--url`, `--api-key`,
`--config` and the generated `--yes`) and mandatory `Short` + `Example`.
Local-only commands (`config`, `skill`, `docs`, help topics) live in
`internal/cli` because they do not touch the server.

### Help text is product

`Short`, `Long` and `Example` are the in-terminal docs: concrete, with real
flag names, accepted value formats and one example per common use.
`-o json` is a contract for scripts and agents: don't reshape it silently.

### Other

- Comments only where the code is not obvious; English everywhere in code.
- Refreshing the embedded spec: `task spec` (it sanitises `servers[].url`;
  `TestEmbeddedSpecIsSanitised` guards it) and bump `spec.EmbeddedVersion`.

## Git conventions

- Branches: `opcli/<feature>`. Conventional prefixes: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`.
- **No `Co-Authored-By: Claude` trailers** in commit messages.
