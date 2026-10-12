---
name: opcli
description: Read and change work in an OpenProject instance from the shell with the `opcli` CLI — list and search work packages, view them with comments and history, create, update, change status along the workflow, comment (with images), relate, watch, attach files, build parent/child hierarchies (epics), log and review time, record work done after the fact (backfill), and manage projects, versions, memberships and notifications; any other API v3 endpoint through `opcli api`. Load whenever the task mentions OpenProject, work packages (WP, #1234), tasks, bugs, epics, milestones, timesheets, time entries, "imputar horas", "apuntar horas", or turning notes into OpenProject work. Use INSTEAD of calling the OpenProject API by hand.
---

# opcli — OpenProject from the shell

`opcli` talks to an **OpenProject** instance (API v3) with the user's own API
key.

```sh
opcli --help · opcli <group> <cmd> --help      # every command has examples
opcli skill show                               # this document
opcli help filters · opcli help output · opcli help auth
```

Configuration lives in `~/.openproject.yaml` (or `OPENPROJECT_URL`,
`OPENPROJECT_API_KEY`, `OPENPROJECT_PROJECT`, `OPENPROJECT_DEFAULT_TYPE`,
`OPENPROJECT_LANGUAGE`): `url` and `api_key` are required; `project` is the
default for commands that need one; `default_type` is the type of new work
packages; `language: es` writes generated comments in Spanish. Never edit the
file or ask for the key yourself: if it is missing or wrong, tell the user to
run `opcli config init --url <url> --api-key <key>` (it also takes
`--project`, `--default-type`, `--language`, `--file`, `--no-verify`).

```sh
opcli whoami -o json          # is the server reachable and the key valid?
opcli config show -o json     # effective configuration (key masked)
```

## Output

Pass **`-o json`** whenever you will read the output. Text output is for humans
and may change. Every command honours `-o json`, writes included.

| Command | JSON |
|---|---|
| any `list` | `{total, count, items: [...]}` — `total > count` means more pages |
| `wp get`, `project get`, `user get`, `meeting get` | one flat record |
| `wp allowed` | `{<field>: [{id, name}, …]}`, e.g. `jq -r '.status[].name'` |
| `wp create`, `wp update`, `wp comment`, `wp comment-edit` | the record written (+ `uploaded[]`) |
| `time log`, `time update`, `wp relation add`, `membership add` | the record created/updated, with its `id` |
| `wp delete`, `time delete`, `notification read`, … | `{ok: true, message}` |
| `wp backfill` | plan or result: `workPackage`, `startDate`, `dueDate`, `totalHours`, `days[]` (`date`, `hours`, `timeEntryId`), `comment` |

Records are flattened from HAL: a link becomes its title plus an id
(`"status": "In progress", "statusId": 7`); Markdown fields are plain strings;
durations stay ISO 8601 (`"estimatedTime": "PT2H"`); work packages carry
`url`, the browser link. `wp list` items are compact (no description). Chain
steps with `jq`:

```sh
ID=$(opcli wp create --subject "Spike: cache" -o json | jq -r .id)
opcli wp list --assignee me --project all -o json | jq -r '.items[] | "\(.id) \(.status) \(.subject)"'
```

## Names instead of ids

| Field | Accepted values |
|---|---|
| project | id, identifier, (partial) name; `all` in list commands |
| status | name or id; in `wp list` also `open` (default), `closed`, `all` |
| type, priority | name or id |
| users | `me`, `none` (unassign), login, email, (partial) name, id |
| version, category | name (needs the project) or id; `none` clears |
| dates | `YYYY-MM-DD`, `today`, `yesterday`, `tomorrow`, `week`, `month`, `+7d`, `-3d`; `none` clears |
| durations | `1.5`, `90m`, `1h30m`, `PT1H30M` |

Names are instance-specific and often localised (a Spanish instance may have
no `Task` or `Closed`). Unknown or ambiguous names fail with the list of valid
values: read it and retry, do not guess twice.

## Find out before you write

```sh
opcli project list -o json
opcli type list --project demo-project -o json    # types enabled in the project
opcli status list -o json
opcli priority list -o json
opcli wp allowed 42 --field status -o json        # transitions valid NOW for this WP
opcli wp allowed 42 -o json                       # every field with a finite list of values
opcli project assignees demo-project -o json      # who can be assigned
opcli project categories demo-project -o json
opcli version list --project demo-project -o json
opcli time activities --wp 42 -o json             # activities valid for time on this WP
opcli role list -o json
opcli user list --search jane -o json             # also --kind group|placeholder|all, --project
opcli user get me -o json
```

Status changes are constrained by workflows (type × role): always check `wp
allowed --field status` before `--status`.

## Read

```sh
opcli wp list --assignee me --project all -o json          # my open work, everywhere
opcli wp list --project demo-project --type Bug --status all --max 100 -o json
opcli wp list --search "login timeout" -o json             # full text: subject, description, comments
opcli wp list --status "In progress,On hold" --sort dueDate:asc -o json
opcli wp list --author me --responsible none --priority High -o json
opcli wp list --version "v1.2" --project demo-project -o json
opcli wp list --parent 42 -o json                          # direct children
opcli wp list --filter '[{"dueDate":{"operator":"<t+","values":["7"]}}]' -o json
opcli wp list --query-id 12 -o json                        # a saved query (see query list)
opcli wp list --max 50 --page 2 -o json
opcli wp get 42 -o json                    # description, relations, children, attachments
opcli wp get 42 --activities -o json       # + comments and change history
opcli wp activities 42 --comments-only -o json
opcli wp relation list 42 -o json
opcli wp watcher list 42 -o json
opcli wp attachment list 42 -o json
opcli wp attachment download 15 --stdout   # read a text attachment
opcli wp attachment download 15 --dest /tmp/report.pdf
opcli query list --project demo-project -o json
opcli notification list --reason mentioned -o json         # unread; --all includes read
opcli notification list --project demo-project --all --max 100 -o json
opcli meeting list --project demo-project -o json
opcli meeting get 7 -o json
```

- `wp list` defaults: open work packages, the configured project (else every
  project), 50 results, most recently updated first. `--max 0` fetches all.
  Flags combine with AND; `--query-id` ignores the other filters.
- `--filter` takes raw API filters for anything without a flag (dates,
  watchers, custom fields…): `opcli help filters` lists operators and names.
- Work packages are `#42` in OpenProject's UI; pass `42` (or `#42`).

## Markdown, images and files

Descriptions and comments are **Markdown**, stored as is: no conversion, no
flag. For long text use a file or stdin instead of a quoted argument; every
text flag has a `-file` twin that accepts a path or `-`:

```sh
opcli wp create --subject "Crash on save" --type Bug --description-file bug.md
git log -1 --format=%B | opcli wp comment 42 --body-file -
```

**Images**: a local image in the Markdown, `![alt](shot.png)`, is uploaded as
an attachment of the work package and the link is rewritten to
`/api/v3/attachments/<id>/content`, so it renders. Relative paths resolve
against the Markdown file (`--description-file`, `--body-file`,
`--comment-file`), or the working directory for inline text and stdin.
Remote URLs, images inside code and absolute paths that do not exist locally
are left alone. Every file is checked before anything is written; a missing
relative path or two different files with the same name are an error.
Re-sending the same text does not attach an image again (same name and size).

**Files**: `--attach` (repeatable) on `wp create` and `wp update`. On create
the files are uploaded first and claimed by the new work package in the same
request: a failed upload leaves no half-made work package, and a rejected
create deletes the files it uploaded. JSON output lists
what was sent in `uploaded[]` (`reused: true` when it was already attached).

## Write

```sh
opcli wp create --project demo-project --subject "Fix login" --type Bug --priority High --assignee me
opcli wp create --subject "Write docs" --due-date +7d --estimate 4h --description-file spec.md --attach mock.png
opcli wp create --subject "Rollout" --start-date 2026-11-02 --due-date 2026-11-06 --version "v1.2" --category Backend
opcli wp create --subject "Spike" --responsible jane --status "In progress" --notify=false
opcli wp update 42 --status "In progress" --assignee me
opcli wp update 42 --status Closed --comment "Fixed in v1.2"
opcli wp update 42 --assignee none --version none --due-date none
opcli wp update 42 --subject "New title" --estimate 6h --remaining 2h --percent-done 60
opcli wp update 42 --project other-project --type Feature
opcli wp update 42 --fields '{"customField3":"ACME"}'
opcli wp update 42 --description-file new.md --comment-file why.md
opcli wp comment 42 --body "Deployed to staging"
opcli wp comment-edit 1234 --body "Deployed to production"
opcli wp relation add 43 --type follows --to 42 --lag 2
opcli wp relation add 43 --type relates --to 30 --description "same root cause"
opcli wp watcher add 42 --user jane
opcli wp watcher remove 42 --user jane
opcli wp attachment upload 42 --file build.log --description "CI output"
opcli notification read 99
opcli notification read --all
```

- `wp update` only sends the flags you pass. Lock versions are handled (one
  automatic retry on a concurrent edit). The fields go in one request: if
  OpenProject rejects one, none changes; `--comment` is posted after it.
- New work packages without `--type` get `default_type` from the config, else
  the project's default type. `--project` falls back to the configured one.
- `--notify` is on by default for create/update: pass `--notify=false` for
  bulk or scripted changes so nobody gets a flood of emails.
- `--fields` merges raw JSON into the request (custom fields, links); read
  the field names with `opcli wp allowed <id>` or `opcli api describe`.
- `wp comment-edit` takes the **activity** id from `wp activities`. Comments
  cannot be deleted through the API.
- `wp relation add 43 --type blocks --to 42`: #43 blocks #42. Types: relates,
  duplicates, duplicated, blocks, blocked, precedes, follows, includes,
  partof, requires, required. Parent/child is not a relation: use `--parent`.

### Epics and hierarchies

An epic is just a type; the hierarchy is the `parent` field:

```sh
EPIC=$(opcli wp create --subject "SSO" --type Epic --description-file epic.md -o json | jq -r .id)
opcli wp create --subject "SAML login" --parent "$EPIC"
opcli wp update 57 --parent 42            # move under #42; --parent none detaches
opcli wp list --parent 42 --status all -o json
```

## Time

```sh
opcli time log --wp 42 --hours 1h30m --activity Development --comment "Code review"
opcli time log --wp 42 --hours 2 --date yesterday
opcli time log --wp 42 --hours 45m --user jane            # on behalf of others: needs permission
opcli time list -o json                                   # mine, this week
opcli time list --from month --group-by project -o json   # also day, wp, activity, user
opcli time list --from 2026-09-01 --to 2026-09-30 --user all --project demo-project -o json
opcli time list --wp 42 --user all --from -365d -o json
opcli time update 15 --hours 2h --comment "Review + fixes"
opcli time update 15 --wp 43 --date 2026-10-09 --activity Testing
```

`--activity` is required by some instances; the valid ones depend on the
project (`time activities --wp <id>`). `time list` returns the hour total.

### Work done but never planned or logged: backfill

`wp backfill` records it in one call: creates the work package (`--subject`)
or reuses one (`--wp`), sets the real start/finish dates (`--from`/`--to`),
spreads `--hours` over the instance's working days (or logs
`--hours-per-day`), applies `--status` last and adds an audit comment with
`--reason`. Always run it first with `--dry-run` and show the plan.

```sh
opcli wp backfill --project demo-project --subject "Hotfix broker" --type Bug --assignee jane --from 2026-09-14 --to 2026-09-18 --hours 12h --activity Development --status Closed --dry-run
opcli wp backfill --wp 42 --from -10d --to -8d --hours-per-day 2h --reason "Done during the incident" --dry-run
opcli wp backfill --wp 42 --from 2026-09-20 --to 2026-09-21 --hours 4h --include-weekends --user jane --comment "On-call"
opcli wp backfill --subject "Audit" --description-file audit.md --parent 40 --estimate 8h --from -3d --hours 6h --reason-file why.md --notify
```

Time goes to `--user`, else the assignee, else you. The creation date of a
work package cannot be changed through the API: reports must filter on
start/finish dates or the time entries' date.

## Projects, versions, members

```sh
opcli project get demo-project -o json
opcli project list --search web --archived -o json
opcli project list --filter '[{"active":{"operator":"=","values":["t"]}}]' -o json
opcli project create --name "Website" --identifier website --parent platform --public --description-file about.md
opcli project update website --status at_risk --status-explanation "Vendor late"
opcli project update website --status-explanation-file status.md
opcli project update website --name "Website 2" --identifier website-2
opcli project update old-stuff --archive
opcli version create --project website --name "v1.2" --start-date today --end-date +14d --status open --sharing none
opcli version update 8 --status closed --description "Released"
opcli membership list --project website --user jane -o json
opcli membership add --project website --user jane --roles "Member,Reader" --message "Welcome"
opcli membership update 31 --roles Reader
```

## Anything else: the raw API

opcli embeds the OpenAPI spec of API v3. For what no command covers (wiki
pages, news, boards, custom actions…):

```sh
opcli api endpoints news                         # find the endpoint
opcli api describe POST /api/v3/news             # parameters and body fields
opcli api request GET /api/v3/grids --query pageSize=5 --flatten -o json
opcli api request POST news --body-file news.json
opcli api describe GET /api/v3/projects --live   # the server's own spec instead
```

Read the contract with `api describe` before a write, and treat POST, PATCH
and PUT as writes: the user must have asked for that change.

## Workflows

Recipes for common requests. Read first, propose, and write only what the user
confirmed.

- **My day**: `wp list --assignee me --project all --sort dueDate:asc --max 100
  -o json`. Flag overdue and due within 3 days, pick 3-5 to focus on, point
  out stale ones (old `updatedAt`). Suggest status changes; do not apply them.
- **Triage #42**: `wp get 42 --activities -o json` + `wp allowed 42 -o json`.
  Check type and priority, what is missing (repro steps, acceptance criteria,
  estimate, dates), propose the next status from the allowed values and an
  assignee, and summarise the discussion. Then `wp update` / `wp comment`.
- **Project status report**: `project get`, open WPs sorted by `dueDate`, and
  WPs updated recently with `--status all --filter
  '[{"updatedAt":{"operator":">t-","values":["7"]}}]'`. Report health, done
  and in progress, overdue and due soon with assignees, risks, next steps.
- **Log my time from free text** ("2h review of the login bug, 1h standup"):
  match each chunk to a WP (my open work first, then `wp list --search`), pick
  an activity from `time activities --wp <id>`, check `time list --from <day>
  --to <day>` for duplicates and days over 10h, show a table, then `time log`.
- **Timesheet review**: `time list --from week --max 0 -o json` (and
  `--group-by day`). Report totals per day and project, missing or short
  days, weekends, vague comments; fix with `time log` / `time update`.
- **Notes or an email into work packages**: search duplicates with `wp list
  --search`, check `type list --project <p>` and `priority list`, draft
  subject (imperative, under 80 chars), type, priority and a Markdown
  description (context; for bugs steps / expected / actual; acceptance
  criteria as a checklist). Split big items into a parent and children.

## Destructive commands

```sh
opcli wp delete 42 --yes                    # children included
opcli time delete 15 --yes
opcli wp relation delete 77 --yes           # ids from 'wp relation list'
opcli wp attachment delete 15 --yes
opcli membership delete 31 --yes
opcli project delete old-stuff --yes        # everything in it
opcli api request DELETE work_packages/42 --yes
```

They ask on a terminal. Without one (your case) they fail with exit 1 unless
you pass `--yes`. Pass it only after the user confirmed that exact deletion:
nothing deleted can be recovered. Removing a watcher is not destructive.

## Not possible with opcli

Say so instead of improvising:

- deleting a comment (the API only allows editing it), changing `createdAt`
  or the author of anything;
- ordering cards on a board or moving them between board columns other than
  by changing the field the column is based on (e.g. `--status`);
- creating or editing meetings, users, groups, roles, types, statuses or
  workflows (admin settings): read-only or web UI;
- custom fields have no dedicated flags: `--fields` with the raw name.

Some of these are reachable through `opcli api` if the user's permissions
allow it: check `api endpoints` and ask before writing.

## Errors

Errors go to stderr as one line (`Error: …`) and exit 1. Only a wrong flag or
argument also prints the command usage.

- `openproject: HTTP n: message (details)`:
  - `422` naming a field (`Status is invalid`, `Type is not set to one of the
    allowed values`): check `wp allowed <id>` / `type list --project`;
  - `422` `Assignee is invalid`: the user is not assignable in that project
    (`project assignees`);
  - `409` after the automatic retry: someone else is editing; re-read and retry once;
  - `401` bad key (the hint says so), `403` no permission, `404` may just mean
    the resource is not visible to the user;
  - `429` and `503` are retried automatically.
- `<kind> "x" is ambiguous; available: …` / `no <kind> matches "x"`: pick one
  of the listed names.
- `cannot reach OpenProject at … (check the URL, the network or the VPN)`: the
  server is not reachable from this machine. Do not retry in a loop or touch
  the config; tell the user.
- `no project given and no default project configured`: pass `--project`.
- `… cannot be undone: pass --yes to confirm`: see Destructive commands.
