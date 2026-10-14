---
name: opcli
description: Read and change work in an OpenProject instance from the shell with the `opcli` CLI — list and search work packages, view them with comments and history, create, update (custom fields by name), change status along the workflow, comment (with images and @mentions that notify), dry-run any write with server-side validation, relate, watch, attach files, build parent/child hierarchies (epics), bulk-update several work packages, set personal reminders, log and review time (or run a timer), record work done after the fact (backfill), schedule meetings with agenda, participants and outcomes (minutes), and manage projects, versions, memberships and notifications; any other API v3 endpoint through `opcli api`. Load whenever the task mentions OpenProject, work packages (WP, #1234), tasks, bugs, epics, milestones, timesheets, time entries, "imputar horas", "apuntar horas", reminders, OpenProject meetings, agendas or minutes ("acta", "orden del día"), or turning notes into OpenProject work. Use INSTEAD of calling the OpenProject API by hand.
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
and may change. Every command honours `-o json`, writes included. Timestamps
are UTC in JSON; meeting and reminder text output shows them in local time.

| Command | JSON |
|---|---|
| any `list` | `{total, count, items: [...]}` — `total > count` means more pages |
| `wp get`, `project get`, `user get` | one flat record |
| `meeting get` | the meeting + `agendaItems[]`, each with its `outcomes[]` |
| `wp allowed` | `{<field>: [{id, name}, …]}`, e.g. `jq -r '.status[].name'` |
| `wp fields` | list of `{key, name, type, required, custom, values?, valuesFrom?, help?}` |
| `wp create`, `wp update`, `wp comment`, `wp comment-edit` | the record written (+ `uploaded[]`, `mentioned[]`, `unresolvedMentions[]`) |
| `wp update` with several ids | a list of the updated work packages |
| any write with `--dry-run` | `{dryRun: true, requests: [{method, path, query?, body?, file?}], mentioned?, unresolvedMentions?}` |
| `time log`, `time update`, `time start`, `time stop`, `wp relation add`, `membership add`, `meeting create`, `meeting agenda add`, `meeting outcome add` | the record created/updated, with its `id` |
| `wp remind` | the reminder (`id`, `remindAt` in UTC, `note`, `remindable`), `replaced: true` if it moved an existing one |
| `time status` | `{running: false}` or the running entry + `running: true`, `elapsed` |
| `wp delete`, `time delete`, `notification read`, … | `{ok: true, message}` |
| `wp flow` | `{project, from, to, startStatuses, throughput, weekly, cycleTime, leadTime, statuses, workPackages}` (see Flow) |
| `wp backfill` | plan or result: `workPackage`, `startDate`, `dueDate`, `totalHours`, `days[]` (`date`, `hours`, `timeEntryId`), `comment` |

Records are flattened from HAL: a link becomes its title plus an id
(`"status": "In progress", "statusId": 7`); Markdown fields are plain strings;
durations stay ISO 8601 (`"estimatedTime": "PT2H"`); work packages carry
`url`, the browser link, and their custom fields by name under
`customFields` (`{"Customer": "ACME", "Tags": ["urgent"]}`; empty ones are
omitted), plus `statusIsClosed` (the status' own flag: the reliable way to
tell open from closed) and `statusCategory` (`todo` = the default status,
`done` = closed, `doing` = any other; a heuristic, OpenProject has no
categories). `wp list` items are compact (no description). Chain steps with `jq`:

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
| date and time | `"YYYY-MM-DD HH:MM"`, `"tomorrow 9:30"`, `"+2d 16:00"`, `16:00` (today), `+2h`, a date alone (09:00); in the user's OpenProject time zone |

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
opcli wp fields --project demo-project --type Task -o json   # fields of a new WP: names, types, required, values
opcli wp fields 42 --custom -o json               # custom fields of this WP and what they accept
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
opcli wp list --project demo-project --field "Customer=ACME" --field "Billable=yes" -o json
opcli wp list --project demo-project --field "Area=Backend,Ops" --field "Due to customer=" -o json   # "NAME=" = empty
opcli wp list --updated-after -7d --status all -o json    # also --updated-before
opcli wp list --created-after month --created-before today -o json
opcli wp list --due-before today -o json                  # overdue (open); also --due-after
opcli wp list -p all --closed-after 2026-09-01 --closed-before 2026-09-30 -o json   # + closedAt
opcli wp list -p all --status all --max 0 -o json         # everything
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
opcli reminder list -o json                                # my active reminders
opcli time status -o json                                  # is a timer running?
```

- `wp list` defaults: open work packages, the configured project (else every
  project), 50 results, most recently updated first. `--max 0` fetches all.
  Flags combine with AND; `--query-id` ignores the other filters.
- `--field NAME=VALUE` filters by any field by its name, custom fields
  included: text matches by "contains", lists, users and versions by name,
  booleans `yes`/`no`, dates as usual. It needs a project. OpenProject cannot
  filter by user or version custom fields: filter by something else and check
  `customFields` with `jq`.
- Date flags take the usual dates (`2026-09-01`, `today`, `week`, `month`,
  `-7d`) and both ends are inclusive. `--due-*` also matches a milestone's
  date.
- OpenProject has no close date: `--closed-after/--closed-before` imply
  `--status closed` (unless you give statuses), read the history of every
  candidate (one request each: those updated since `--closed-after`) and keep
  the ones whose last close (entry into a closed status) is in range, adding
  `closedAt` (UTC). Not with `--page`.
- `--max 0` reads 1000 per page, several pages at once; tens of thousands of
  work packages take minutes. It pages in creation order (updates made
  meanwhile would otherwise make work packages jump pages and get lost) and then applies
  `--sort` locally for id, updatedAt, createdAt, startDate, dueDate and
  subject; if fewer than `total` come back it warns on stderr. If the server fails to render a page (HTTP 5xx,
  typically a type whose form configuration is broken), opcli re-reads it by
  id and warns on stderr (`opcli: warning: …`) with what it skipped; a
  type whose schema fails keeps its custom fields as `customFieldN`, also
  warned. stdout stays valid JSON: check stderr before trusting a total.
- `--filter` takes raw API filters for anything without a flag (watchers,
  relative dates…): `opcli help filters` lists operators and names.
- Work packages are `#42` in OpenProject's UI; pass `42` (or `#42`).

### Flow: throughput and cycle time

```sh
opcli wp flow -p demo-project -o json                    # closed in the last 30 days
opcli wp flow -p all --since 12w --type Task -o json
opcli wp flow -p demo-project --since 2026-07-01 --until 2026-09-30 -o json
opcli wp flow -p demo-project --start-status "In progress" --assignee me --list
```

`wp flow` measures the work packages closed in the period (`--since`, default
`30d`; also `12w` or a date; `--until`, default today): those whose current
status is closed and that entered it (the final run of closed statuses)
within the period. JSON: `throughput`; `weekly: [{week (Monday), count,
partial?}]`; `cycleTime` and `leadTime`: `{count, p50, p85, max}` in calendar
days (cycle: first entry into a start status → close; lead: creation →
close); `statuses: [{name, days, share (%), perWP: {count, p50, p85, max}}]`,
where the cycle time went; `workPackages: [{id, subject, type, project,
status, assignee, created, started?, closed, cycleDays?, leadDays, url,
history: [{status, entered, left?, days?}]}]`, every status the work package
went through with entry and exit (UTC; the last one has no `left`). Start
statuses default to the `doing` ones; `--start-status` picks others. Closed
without a start status: throughput only. Statuses are read from each work
package's activities whatever the user's language. `--list` lists the work
packages in text output.

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
opcli wp create --subject "Onboarding" --field "Customer=ACME" --field "Area=Backend" --field "Billable=yes"
opcli wp update 42 --field "Tags=urgent,customer" --field "Points=8" --field "Due to customer=+14d"
opcli wp update 42 --field "Customer="               # clear a custom field
opcli wp update 42 --status Closed --dry-run         # validated by the server, nothing written
opcli wp update 42 --description-file new.md --comment-file why.md
opcli wp update 42 43 44 --version "Sprint 7" --notify=false --dry-run   # same change on several
opcli wp remind 42 --at "tomorrow 9:00" --note "Call the customer"
opcli wp comment 42 --body "Deployed to staging"
opcli wp comment 42 --body "@jane can you review the migration?"
opcli wp comment 42 --body "Customer budget is tight" --internal
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
- `wp update` with **several ids** (`42 43 44` or `42,43`) applies the same
  change to each in turn, each validated on its own. If any fails, the error
  names those that failed and why and those that were updated: nothing is
  rolled back. Dry-run it first.
- `wp remind` sets a **personal reminder**: OpenProject notifies only you at
  that time. One active reminder per work package: setting another moves it
  (`replaced: true`). The time must be in the future. `reminder delete <id>`
  cancels it.
- New work packages without `--type` get `default_type` from the config, else
  the project's default type. `--project` falls back to the configured one.
- `--notify` is on by default for create/update: pass `--notify=false` for
  bulk or scripted changes so nobody gets a flood of emails.
- **Custom fields** go by the name the UI shows: `--field NAME=VALUE`,
  repeatable. `opcli wp fields` lists the names, types, whether they are
  required and the allowed values (and the instance's help text for each).
  Values are converted by type: lists and multi-lists by option name
  (`"Tags=a,b"`), users like `--assignee`, versions by name, numbers (`3,5` or
  `3.5`), booleans (`yes`/`no`, `true`/`false`, `sí`), dates and durations in
  the usual formats; `"NAME="` clears. A raw JSON value (`[…]`, `{…}`) is
  sent as is. `--fields` merges raw JSON into the request with API keys
  (`customField3`); prefer `--field`.
- **Every create and update is validated by the server first** (its form
  endpoint), before any file is uploaded: a status the workflow does not allow
  from the current one, a missing required field or a value that is not
  allowed fails with `the server would reject this, nothing was written` and
  the allowed values. Read them and retry with one of them.
- **`--dry-run`** on any write prints the requests it would send and sends
  none; reads and that server-side validation still run, so a clean dry run of
  `wp create`/`wp update` means the server would accept it. Show it to the
  user before a write they have not confirmed in detail. Files are listed,
  not uploaded.
- **Mentions**: `@login` in a description or comment becomes a real mention
  that notifies the user; a plain "@name" in OpenProject notifies nobody. It
  also accepts a first name, last name or full name without spaces when it
  names exactly one user (only admins can see logins). What matches nobody,
  or more than one user, is left as typed and listed in
  `unresolvedMentions`: fix the name and edit the text, or tell the user.
  `@` inside code and in email addresses is left alone.
- `--internal` (on `wp comment`, and on `wp update` for its `--comment`)
  makes the comment visible only to roles allowed to see internal comments.
  It needs an Enterprise plan and the project setting; otherwise the server
  says so.
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
opcli time start 42 --activity Development --comment "Code review"   # timer
opcli time status -o json
opcli time stop                                           # logs the elapsed time
opcli time stop --hours 1h15m --comment "Review + fixes"  # or a different amount
```

`--activity` is required by some instances; the valid ones depend on the
project (`time activities --wp <id>`). `time list` returns the hour total.
`time log` and `time update` are validated by the server before writing.

The **timer** is the one the web UI shows: one per user at a time.
`time start` refuses while another runs (it names it); `time stop` turns it
into a time entry with the elapsed time rounded to the minute, or `--hours`.

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

## Meetings

A meeting has a start, a duration, a location, participants, a state and an
agenda. OpenProject does not create the video call: create it in Google Meet
or Teams and put the link in `--location`.

```sh
opcli meeting list --when upcoming --participant me -o json
opcli meeting list --project demo-project --when past --max 10 -o json
opcli meeting get 7 -o json                 # participants, agenda items, outcomes
MID=$(opcli meeting create --project demo-project --title "Sprint review" --start "2026-10-20 10:00" --duration 45m --location https://meet.google.com/abc-defg-hij --participant jane,john -o json | jq -r .id)
opcli meeting agenda add "$MID" --title "Release status" --duration 10m --presenter jane --notes-file status.md
opcli meeting agenda add "$MID" --wp 42 --duration 5m      # discuss a work package
opcli meeting agenda update 37 --title "Release status and risks" --duration 15m --presenter none --notes "Moved to Friday"
opcli meeting agenda update 38 --wp 43
opcli meeting update "$MID" --state open --notify          # publish it and email the participants
opcli meeting update "$MID" --title "Sprint 7 review" --start "2026-10-21 10:00" --participant ana --remove-participant john
opcli meeting update "$MID" --state in_progress            # needed before recording outcomes
opcli meeting outcome add 37 --notes "Release moves to Friday"
opcli meeting outcome add 37 --wp 51 --notes "Jane prepares the migration"   # follow-up WP
opcli meeting update "$MID" --state closed
```

- You are added to the participants of a meeting you create; `meeting update
  --participant` adds and `--remove-participant` removes.
- States: `draft` (new meetings) → `open` → `in_progress` → `closed`, or
  `cancelled`. Outcomes can only be recorded `in_progress`; a closed meeting's
  agenda cannot change. `--notify` emails the participants about that change.
- `meeting create` and `meeting update` are validated by the server first.
- Agenda item and outcome ids come from `meeting get`. A follow-up work
  package is created with `wp create` first, then linked with `outcome add --wp`.

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
  WPs updated recently with `--status all --updated-after -7d`; `wp flow
  --since 4w` for throughput and cycle time. Report health, done
  and in progress, overdue and due soon with assignees, risks, next steps.
- **Log my time from free text** ("2h review of the login bug, 1h standup"):
  match each chunk to a WP (my open work first, then `wp list --search`), pick
  an activity from `time activities --wp <id>`, check `time list --from <day>
  --to <day>` for duplicates and days over 10h, show a table, then `time log`.
- **Timesheet review**: `time list --from week --max 0 -o json` (and
  `--group-by day`). Report totals per day and project, missing or short
  days, weekends, vague comments; fix with `time log` / `time update`.
- **Meeting minutes**: `meeting get <id> -o json`; turn the notes the user
  gives you into outcomes per agenda item (`outcome add --notes`), create a
  work package for each action item and link it (`outcome add --wp`), then
  propose closing the meeting. Show the list before writing.
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
opcli meeting delete 7 --yes                # with its agenda and outcomes
opcli meeting agenda delete 37 --yes
opcli meeting outcome delete 4 --yes
opcli reminder delete 3 --yes
opcli project delete old-stuff --yes        # everything in it
opcli api request DELETE work_packages/42 --yes
```

They ask on a terminal. Without one (your case) they fail with exit 1 unless
you pass `--yes`. `--dry-run` shows what would be deleted without asking. Pass it only after the user confirmed that exact deletion:
nothing deleted can be recovered. Removing a watcher is not destructive.

## Not possible with opcli

Say so instead of improvising:

- deleting a comment (the API only allows editing it), changing `createdAt`
  or the author of anything;
- ordering cards on a board or moving them between board columns other than
  by changing the field the column is based on (e.g. `--status`);
- recurring meeting series and meeting sections (reachable with `opcli api
  endpoints meeting`); creating the Google Meet / Teams call itself;
- creating or editing users, groups, roles, types, statuses, custom field
  definitions or workflows (admin settings): read-only or web UI;
- filtering by user or version custom fields (OpenProject has no filter for
  them).

Some of these are reachable through `opcli api` if the user's permissions
allow it: check `api endpoints` and ask before writing.

## Errors

Errors go to stderr as one line (`Error: …`) and exit 1. Only a wrong flag or
argument also prints the command usage. Lines starting `opcli: warning:` are
not errors: the command finished but lost or degraded something (a page
re-read by id, a skipped work package, custom fields without names). Pass
them on to the user.

- `openproject: HTTP 422: the server would reject this, nothing was written
  (… Allowed: a, b.)`: the form check of a create/update failed; nothing
  changed. Pick one of the allowed values (or fill the required field).
- `field "X": "y" is not allowed; allowed: …` / `unknown field "X" here;
  custom fields: …`: a `--field` name or value; see `wp fields`.
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
- `outcomes can only change while the meeting is in progress: opcli meeting
  update N --state in_progress`: run that (if the meeting is really being
  held) and retry; adding and deleting outcomes both need it.
- `a timer is already running on #N …`: `time stop` it first (ask the user if
  it is not yours to stop); `no timer is running`: nothing to stop.
- `Date must be in the future` on `wp remind`: the time is past (times are in
  the user's OpenProject time zone).
- `… cannot be undone: pass --yes to confirm`: see Destructive commands.
