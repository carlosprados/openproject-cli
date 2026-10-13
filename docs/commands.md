# opcli command reference

Generated with `opcli docs`. Global flags: `--config`, `--url`, `--api-key`, `-o text|json`. Destructive commands also take `--yes`.

| Command | Effect | Summary |
|---|---|---|
| `opcli api describe` | read-only | Show parameters, body fields and examples of an endpoint |
| `opcli api endpoints` | read-only | Search the catalogue of API v3 endpoints |
| `opcli api request` | **destructive** | Call any API v3 endpoint and print the JSON answer |
| `opcli meeting agenda add` | writes | Add an agenda item (free text or a work package) to a meeting |
| `opcli meeting agenda delete` | **destructive** | Remove an agenda item |
| `opcli meeting agenda update` | writes | Change an agenda item |
| `opcli meeting create` | writes | Schedule a meeting |
| `opcli meeting delete` | **destructive** | Delete a meeting with its agenda and outcomes |
| `opcli meeting get` | read-only | Show a meeting with its participants, agenda items and outcomes |
| `opcli meeting list` | read-only | List meetings |
| `opcli meeting outcome add` | writes | Record an outcome (decision or follow-up work package) on an agenda item |
| `opcli meeting outcome delete` | **destructive** | Delete an outcome |
| `opcli meeting update` | writes | Reschedule a meeting, change its participants or move it along its states |
| `opcli membership add` | writes | Add a user or group to a project with one or more roles |
| `opcli membership delete` | **destructive** | Remove a membership (the user loses access to the project) |
| `opcli membership list` | read-only | List memberships, optionally by project and/or user |
| `opcli membership update` | writes | Replace the roles of a membership |
| `opcli notification list` | read-only | List your notifications (unread by default) |
| `opcli notification read` | writes | Mark one notification (by id) or all unread notifications as read |
| `opcli priority list` | read-only | List work package priorities |
| `opcli project assignees` | read-only | List users that can be assigned work packages in a project |
| `opcli project categories` | read-only | List work package categories of a project |
| `opcli project create` | writes | Create a project |
| `opcli project delete` | **destructive** | Delete a project and everything in it (irreversible) |
| `opcli project get` | read-only | Show a project |
| `opcli project list` | read-only | List projects |
| `opcli project update` | writes | Update a project (name, description, status, archive...) |
| `opcli query list` | read-only | List saved work package queries; run one with 'opcli wp list --query-id <id>' |
| `opcli reminder delete` | **destructive** | Delete one of your reminders |
| `opcli reminder list` | read-only | List your active reminders |
| `opcli role list` | read-only | List roles |
| `opcli status list` | read-only | List all work package statuses |
| `opcli time activities` | read-only | List activities valid for time entries (optionally for a work package) |
| `opcli time delete` | **destructive** | Delete a time entry |
| `opcli time list` | read-only | List time entries (yours by default) with an hour total |
| `opcli time log` | writes | Log time on a work package |
| `opcli time start` | writes | Start a timer on a work package |
| `opcli time status` | read-only | Show the running timer, if any |
| `opcli time stop` | writes | Stop the running timer and log the elapsed time |
| `opcli time update` | writes | Update a time entry |
| `opcli type list` | read-only | List work package types, globally or enabled in a project |
| `opcli user get` | read-only | Show a user |
| `opcli user list` | read-only | List or search users, groups and placeholder users |
| `opcli version create` | writes | Create a version in a project |
| `opcli version list` | read-only | List versions of a project (or all visible versions with --project all) |
| `opcli version update` | writes | Update a version (dates, status, name...) |
| `opcli whoami` | read-only | Show the authenticated user (checks URL and API key) |
| `opcli wp activities` | read-only | Show comments and change history of a work package |
| `opcli wp allowed` | read-only | Show values a work package accepts: statuses, types, priorities, versions... |
| `opcli wp attachment delete` | **destructive** | Delete an attachment by its id |
| `opcli wp attachment download` | read-only | Download an attachment by its id |
| `opcli wp attachment list` | read-only | List attachments of a work package |
| `opcli wp attachment upload` | writes | Attach a local file to a work package |
| `opcli wp backfill` | writes | Record work done after the fact: real dates, hours per day and an audit comment |
| `opcli wp comment` | writes | Add a comment to a work package |
| `opcli wp comment-edit` | writes | Replace the text of a comment |
| `opcli wp create` | writes | Create a work package |
| `opcli wp delete` | **destructive** | Delete a work package (irreversible, children included) |
| `opcli wp fields` | read-only | List the fields a work package takes: names, types, required, allowed values |
| `opcli wp get` | read-only | Show a work package with description, relations, children and attachments |
| `opcli wp list` | read-only | List and search work packages with filters |
| `opcli wp relation add` | writes | Relate two work packages |
| `opcli wp relation delete` | **destructive** | Delete a relation by its id (see 'wp relation list') |
| `opcli wp relation list` | read-only | List relations of a work package |
| `opcli wp remind` | writes | Remind yourself about a work package at a given time |
| `opcli wp update` | writes | Update fields of a work package (status, assignee, dates, ...) |
| `opcli wp watcher add` | writes | Add a watcher to a work package |
| `opcli wp watcher list` | read-only | List watchers of a work package |
| `opcli wp watcher remove` | writes | Remove a watcher from a work package |

## opcli api describe

Show parameters, body fields and examples of an endpoint

Describe an endpoint from the OpenAPI spec. The path can be the template
(/api/v3/work_packages/{id}) or a concrete path (/api/v3/work_packages/42).
Omit the method to describe every method of the path.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<method-or-path>` (arg) | string |  | **required** HTTP method, or the path when describing all methods |
| `<path>` (arg) | string |  | Endpoint path (when a method is given first) |
| `--live` | bool |  | Use the spec served by the configured instance instead of the embedded one |

```sh
  opcli api describe /api/v3/time_entries
  opcli api describe POST /api/v3/time_entries
  opcli api describe PATCH work_packages/42 -o json
```

## opcli api endpoints

Search the catalogue of API v3 endpoints

List endpoints whose method, path, tag or summary contain every given word.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<search>` (arg) | string |  | Words to match (all must match) |
| `--live` | bool |  | Use the spec served by the configured instance instead of the embedded one |

```sh
  opcli api endpoints                 # all ~300 operations
  opcli api endpoints meeting
  opcli api endpoints post work_packages
  opcli api endpoints grids --live
```

## opcli api request

Call any API v3 endpoint and print the JSON answer

Perform an authenticated request. The path may be relative to /api/v3
("work_packages/42") or absolute ("/api/v3/work_packages/42"). Query
parameters go in --query key=value (repeatable); JSON values such as
filters are passed verbatim. --flatten simplifies HAL answers (links become
"name"/"nameId" pairs, collections become their elements).

DELETE asks for confirmation (--yes without a terminal). POST, PATCH and
PUT write too: read the contract with 'opcli api describe' first.

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<method>` (arg) | string |  | **required** HTTP method |
| `<path>` (arg) | string |  | **required** Endpoint path, e.g. /api/v3/projects or projects/3 |
| `--query` | list |  | Query parameter key=value (repeatable) |
| `--body` | string |  | JSON request body |
| `--flatten` | bool |  | Simplify HAL output |

```sh
  opcli api request GET /api/v3/configuration
  opcli api request GET work_packages --query 'filters=[{"status":{"operator":"o","values":[]}}]' --query pageSize=5 --flatten
  opcli api request POST /api/v3/render/markdown --body '"**bold**"'
  opcli api request PATCH work_packages/42 --body '{"lockVersion":3,"subject":"New"}'
  opcli api request POST news --body-file news.json
```

## opcli meeting agenda add

Add an agenda item (free text or a work package) to a meeting

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Meeting id |
| `--title` | string |  | Title (optional for a work package item) |
| `--wp` | string |  | Discuss this work package (id) |
| `--duration` | string |  | Time slot: 10m, 1h, 0.5 |
| `--notes` | string |  | Notes (Markdown) |
| `--presenter` | string |  | Who presents it: me, login, email, name or id ('none' clears) |

```sh
  opcli meeting agenda add 7 --title "Release status" --duration 10m --presenter jane.doe
  opcli meeting agenda add 7 --wp 42 --duration 5m
  opcli meeting agenda add 7 --title "Risks" --notes-file risks.md
```

## opcli meeting agenda delete

Remove an agenda item

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Agenda item id |

```sh
  opcli meeting agenda delete 37 --yes
```

## opcli meeting agenda update

Change an agenda item

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Agenda item id |
| `--title` | string |  | Title (optional for a work package item) |
| `--wp` | string |  | Discuss this work package (id) |
| `--duration` | string |  | Time slot: 10m, 1h, 0.5 |
| `--notes` | string |  | Notes (Markdown) |
| `--presenter` | string |  | Who presents it: me, login, email, name or id ('none' clears) |

```sh
  opcli meeting agenda update 37 --duration 15m --notes "Moved to Friday"
  opcli meeting agenda update 37 --presenter none
```

## opcli meeting create

Schedule a meeting

Schedule a one-off meeting in a project. You are added to the participants;
--participant invites others. New meetings start as a draft; '--state open'
opens it at once. The server validates the meeting before it is created.

Add the agenda afterwards with 'opcli meeting agenda add'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name (default: configured project) |
| `--participant` | list |  | Invite these users (me, login, email, name or id) |
| `--state` | string |  | Initial state (default: draft) |
| `--title` | string |  | Title |
| `--start` | string |  | Start: "YYYY-MM-DD HH:MM", "tomorrow 9:30", "16:00", "+2h" (your OpenProject time zone) |
| `--duration` | string |  | Duration: 45m, 1h30m, 1.5 (hours) |
| `--location` | string |  | Room or video call link (Google Meet, Teams, ...) |
| `--notify` | bool |  | Email the participants about this change |

```sh
  opcli meeting create --title "Sprint review" --start "2026-10-20 10:00" --duration 45m \
    --location https://meet.google.com/abc-defg-hij --participant jane.doe,john.roe
  opcli meeting create -p demo-project --title "Kick-off" --start "tomorrow 9:30" --state open --notify
```

## opcli meeting delete

Delete a meeting with its agenda and outcomes

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Meeting id |

```sh
  opcli meeting delete 7 --yes
```

## opcli meeting get

Show a meeting with its participants, agenda items and outcomes

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Meeting id |

```sh
  opcli meeting get 3
```

## opcli meeting list

List meetings

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Only meetings of this project (default: all visible) |
| `--when` | string |  | upcoming (not yet ended, soonest first) or past (started before now, latest first) |
| `--participant` | string |  | Only meetings this user is invited to: me, login, email, name or id |
| `--max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli meeting list --when upcoming
  opcli meeting list --when upcoming --participant me
  opcli meeting list -p demo-project --when past --max 10
```

## opcli meeting outcome add

Record an outcome (decision or follow-up work package) on an agenda item

Record what came out of an agenda item: --notes for a decision or
information, --wp to link a follow-up work package (create it first with
'opcli wp create'). The meeting must be in progress
('opcli meeting update <id> --state in_progress').

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Agenda item id |
| `--notes` | string |  | What was decided (Markdown) |
| `--wp` | string |  | Follow-up work package id |

```sh
  opcli meeting outcome add 37 --notes "Release moves to Friday"
  opcli meeting outcome add 37 --wp 51 --notes "Jane prepares the migration"
```

## opcli meeting outcome delete

Delete an outcome

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Outcome id |

```sh
  opcli meeting outcome delete 2 --yes
```

## opcli meeting update

Reschedule a meeting, change its participants or move it along its states

Change only what you pass. --participant adds users and
--remove-participant removes them. --state moves the meeting along
draft → open → in_progress → closed (or cancelled); outcomes can only be
recorded while it is in_progress.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Meeting id |
| `--participant` | list |  | Add these users (me, login, email, name or id) |
| `--remove-participant` | list |  | Remove these users |
| `--state` | string |  | New state |
| `--title` | string |  | Title |
| `--start` | string |  | Start: "YYYY-MM-DD HH:MM", "tomorrow 9:30", "16:00", "+2h" (your OpenProject time zone) |
| `--duration` | string |  | Duration: 45m, 1h30m, 1.5 (hours) |
| `--location` | string |  | Room or video call link (Google Meet, Teams, ...) |
| `--notify` | bool |  | Email the participants about this change |

```sh
  opcli meeting update 7 --start "2026-10-21 10:00" --notify
  opcli meeting update 7 --participant jane.doe --remove-participant john.roe
  opcli meeting update 7 --state in_progress
  opcli meeting update 7 --location https://teams.microsoft.com/l/meetup-join/...
```

## opcli membership add

Add a user or group to a project with one or more roles

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name (default: configured project) |
| `--user` | string |  | **required** User/group: login, email, name or id |
| `--roles` | list |  | **required** Role names or ids (see 'opcli role list') |
| `--message` | string |  | Custom text for the notification email |

```sh
  opcli membership add -p demo-project --user jane.doe --roles Member
  opcli membership add -p demo-project --user devs --roles "Member,Reader" --message "Welcome!"
```

## opcli membership delete

Remove a membership (the user loses access to the project)

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Membership id |

```sh
  opcli membership delete 12
```

## opcli membership list

List memberships, optionally by project and/or user

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name (default: all projects) |
| `--user` | string |  | me, or login/email/name/id |
| `--max` | int | `200` | Maximum number of results (0 = all) |

```sh
  opcli membership list -p demo-project
  opcli membership list --user me
```

## opcli membership update

Replace the roles of a membership

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Membership id |
| `--roles` | list |  | **required** Role names or ids |

```sh
  opcli membership update 12 --roles "Project admin"
```

## opcli notification list

List your notifications (unread by default)

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--all` | bool |  | Include already read notifications |
| `--reason` | string |  | Only this reason: mentioned, assigned, responsible, watched, commented, created, processed, prioritized, scheduled, dateAlert, shared |
| `--project` | string |  | Only notifications of this project |
| `--max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli notification list
  opcli notification list --all --max 100
  opcli notification list --reason mentioned
```

## opcli notification read

Mark one notification (by id) or all unread notifications as read

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | Notification id (omit with --all) |
| `--all` | bool |  | Mark every unread notification as read |

```sh
  opcli notification read 123
  opcli notification read --all
```

## opcli priority list

List work package priorities

Effect: read-only

```sh
  opcli priority list
```

## opcli project assignees

List users that can be assigned work packages in a project

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<project>` (arg) | string |  | Project id, identifier or name (default: configured project) |

```sh
  opcli project assignees demo-project
```

## opcli project categories

List work package categories of a project

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<project>` (arg) | string |  | Project id, identifier or name (default: configured project) |

```sh
  opcli project categories demo-project
```

## opcli project create

Create a project

Create a project. The identifier is derived from the name when omitted.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--name` | string |  | **required** Project name |
| `--identifier` | string |  | URL identifier (lowercase, digits, dashes) |
| `--description` | string |  | Description (Markdown) |
| `--parent` | string |  | Parent project id/identifier/name ('none' for top level) |
| `--public` | bool |  | Visible to everybody |
| `--status` | string |  | Project status: not_started, on_track, at_risk, off_track, finished, discontinued |
| `--status-explanation` | string |  | Status explanation (Markdown) |

```sh
  opcli project create --name "Website relaunch"
  opcli project create --name "Backend" --identifier web-backend --parent website-relaunch --public
```

## opcli project delete

Delete a project and everything in it (irreversible)

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<project>` (arg) | string |  | **required** Project id, identifier or name |

```sh
  opcli project delete old-stuff
```

## opcli project get

Show a project

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<project>` (arg) | string |  | **required** Project id, identifier or name |

```sh
  opcli project get demo-project
  opcli project get 3 -o json
```

## opcli project list

List projects

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--search` | string |  | Filter by name or identifier (substring) |
| `--archived` | bool |  | List archived projects instead of active ones |
| `--filter` | string |  | Extra raw API filters as a JSON array |
| `--max` | int | `100` | Maximum number of results (0 = all) |
| `--page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli project list
  opcli project list --search radar
  opcli project list --archived
```

## opcli project update

Update a project (name, description, status, archive...)

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<project>` (arg) | string |  | **required** Project id, identifier or name |
| `--name` | string |  | New name |
| `--identifier` | string |  | New identifier |
| `--archive` | bool |  | Archive the project (true) or unarchive it (false) |
| `--description` | string |  | Description (Markdown) |
| `--parent` | string |  | Parent project id/identifier/name ('none' for top level) |
| `--public` | bool |  | Visible to everybody |
| `--status` | string |  | Project status: not_started, on_track, at_risk, off_track, finished, discontinued |
| `--status-explanation` | string |  | Status explanation (Markdown) |

```sh
  opcli project update demo-project --status at_risk --status-explanation "Vendor late"
  opcli project update 3 --name "New name"
  opcli project update old-stuff --archive
```

## opcli query list

List saved work package queries; run one with 'opcli wp list --query-id <id>'

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Only queries of this project (default: all visible) |

```sh
  opcli query list
  opcli query list -p demo-project
  opcli wp list --query-id 5
```

## opcli reminder delete

Delete one of your reminders

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Reminder id |

```sh
  opcli reminder delete 3 --yes
```

## opcli reminder list

List your active reminders

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--max` | int | `100` | Maximum number of results (0 = all) |

```sh
  opcli reminder list
```

## opcli role list

List roles

Effect: read-only

```sh
  opcli role list
```

## opcli status list

List all work package statuses

List every status. Which ones a given work package may move to depends on the workflow: use 'opcli wp allowed <id> --field status'.

Effect: read-only

```sh
  opcli status list
```

## opcli time activities

List activities valid for time entries (optionally for a work package)

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--wp` | string |  | Work package id (activities can be restricted per project) |

```sh
  opcli time activities
  opcli time activities --wp 42
```

## opcli time delete

Delete a time entry

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Time entry id |

```sh
  opcli time delete 17
```

## opcli time list

List time entries (yours by default) with an hour total

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--user` | string | `me` | me (default), all, or login/email/name/id |
| `--project` | string |  | Project id, identifier or name (default: all projects) |
| `--wp` | string |  | Only entries of this work package id |
| `--from` | string | `week` | First day (inclusive): YYYY-MM-DD, today, yesterday, week, month, -Nd |
| `--to` | string | `today` | Last day (inclusive) |
| `--group-by` | string |  | Summarise hours by day, project, wp, activity or user instead of listing entries |
| `--max` | int | `500` | Maximum number of results (0 = all) |

```sh
  opcli time list                          # my entries, this week
  opcli time list --from month             # my entries, this month
  opcli time list --from 2026-09-01 --to 2026-09-30 --user all -p demo-project
  opcli time list --wp 42 --user all --from -365d
  opcli time list --group-by day
```

## opcli time log

Log time on a work package

Log hours on a work package. The project is taken from the work package.
--activity is required by some instances; list valid ones with
'opcli time activities --wp <id>'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--wp` | string |  | **required** Work package id |
| `--hours` | string |  | **required** Time spent: 1.5, 90m, 1h30m, PT1H30M |
| `--date` | string | `today` | Day the work was done (YYYY-MM-DD, today, yesterday, -Nd) |
| `--comment` | string |  | What was done |
| `--activity` | string |  | Activity name or id (Development, Management, ...) |
| `--user` | string |  | Log on behalf of this user (needs permission) |

```sh
  opcli time log --wp 42 --hours 1.5 --comment "Code review"
  opcli time log --wp 42 --hours 2h --date yesterday --activity Development
  opcli time log --wp 42 --hours 45m --user jane.doe      # admins: on behalf of others
```

## opcli time start

Start a timer on a work package

Start tracking time on a work package now. 'opcli time stop' logs the
elapsed time. Only one timer runs at a time, and it is the same timer the
web UI shows: stop the running one first.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<wp>` (arg) | string |  | **required** Work package id |
| `--comment` | string |  | What you are doing |
| `--activity` | string |  | Activity name or id |

```sh
  opcli time start 42
  opcli time start 42 --activity Development --comment "Code review"
```

## opcli time status

Show the running timer, if any

Effect: read-only

```sh
  opcli time status
```

## opcli time stop

Stop the running timer and log the elapsed time

Stop the running timer: it becomes a time entry with the time elapsed
since it started, rounded to the minute. --hours logs a different amount;
--comment and --activity fill what the start left empty.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--hours` | string |  | Log this instead of the elapsed time: 1.5, 90m, 1h30m |
| `--comment` | string |  | What was done |
| `--activity` | string |  | Activity name or id |

```sh
  opcli time stop
  opcli time stop --comment "Reviewed the PR" --activity Development
  opcli time stop --hours 1h15m
```

## opcli time update

Update a time entry

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Time entry id |
| `--wp` | string |  | Move to this work package id |
| `--hours` | string |  | Time spent: 1.5, 90m, 1h30m, PT1H30M |
| `--date` | string |  | Day the work was done |
| `--comment` | string |  | What was done |
| `--activity` | string |  | Activity name or id |

```sh
  opcli time update 17 --hours 3 --comment "Longer than expected"
  opcli time update 17 --date 2026-09-24 --wp 43
```

## opcli type list

List work package types, globally or enabled in a project

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Only types enabled in this project (default: all types) |

```sh
  opcli type list
  opcli type list -p demo-project
```

## opcli user get

Show a user

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<user>` (arg) | string |  | **required** me, or login/email/name/id |

```sh
  opcli user get me
  opcli user get jane.doe
  opcli user get 8 -o json
```

## opcli user list

List or search users, groups and placeholder users

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--search` | string |  | Match name, login or email (substring) |
| `--kind` | string | `user` | user (default), group, placeholder or all |
| `--project` | string |  | Only members of this project |
| `--max` | int | `100` | Maximum number of results (0 = all) |
| `--page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli user list
  opcli user list --search jane
  opcli user list --kind group
  opcli user list -p demo-project           # members of a project
```

## opcli version create

Create a version in a project

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name (default: configured project) |
| `--name` | string |  | **required** Version name |
| `--description` | string |  | Description |
| `--start-date` | string |  | Start date (YYYY-MM-DD, today, +7d...) |
| `--end-date` | string |  | Finish date (YYYY-MM-DD, +14d...) |
| `--status` | string |  | open, locked or closed |
| `--sharing` | string |  | Share with: none, descendants, hierarchy, tree, system |

```sh
  opcli version create -p demo-project --name "v1.2" --end-date 2026-11-30
```

## opcli version list

List versions of a project (or all visible versions with --project all)

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name; 'all' for every project |

```sh
  opcli version list -p demo-project
  opcli version list --project all
```

## opcli version update

Update a version (dates, status, name...)

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Version id |
| `--name` | string |  | New name |
| `--description` | string |  | Description |
| `--start-date` | string |  | Start date (YYYY-MM-DD, today, +7d...) |
| `--end-date` | string |  | Finish date (YYYY-MM-DD, +14d...) |
| `--status` | string |  | open, locked or closed |
| `--sharing` | string |  | Share with: none, descendants, hierarchy, tree, system |

```sh
  opcli version update 4 --status closed
```

## opcli whoami

Show the authenticated user (checks URL and API key)

Effect: read-only

```sh
  opcli whoami
  opcli whoami -o json
```

## opcli wp activities

Show comments and change history of a work package

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--comments-only` | bool |  | Only entries with a comment |

```sh
  opcli wp activities 42
  opcli wp activities 42 --comments-only
```

## opcli wp allowed

Show values a work package accepts: statuses, types, priorities, versions...

Ask OpenProject which values each field of a work package accepts right now.
Statuses reflect the workflow (valid transitions for this type and your role).
Without --field all fields with a finite list of values are shown. Pass
--field assignee to list the users that can be assigned.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--field` | string |  | Only this field (status, type, priority, category, version, assignee, responsible, project, projectPhase) |

```sh
  opcli wp allowed 42 --field status
  opcli wp allowed 42 --field assignee
  opcli wp allowed 42 -o json
```

## opcli wp attachment delete

Delete an attachment by its id

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Attachment id |

```sh
  opcli wp attachment delete 15
```

## opcli wp attachment download

Download an attachment by its id

Download an attachment (ids come from 'wp attachment list' or 'wp get').
The file is written to --dest (default: its original name in the current
directory). With --stdout text content is returned instead of saved, which
is how an AI agent can read a text attachment.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Attachment id |
| `--dest` | string |  | Destination path (default: original file name) |
| `--stdout` | bool |  | Return the content instead of writing a file (text files only) |

```sh
  opcli wp attachment download 15
  opcli wp attachment download 15 --dest /tmp/log.txt
  opcli wp attachment download 15 --stdout | less
```

## opcli wp attachment list

List attachments of a work package

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |

```sh
  opcli wp attachment list 42
```

## opcli wp attachment upload

Attach a local file to a work package

Upload one file. To attach several at once, or while creating or
updating a work package, use --attach on 'wp create' / 'wp update'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--file` | string |  | **required** Path of the file to upload |
| `--description` | string |  | Optional description |

```sh
  opcli wp attachment upload 42 --file ./screenshot.png
  opcli wp attachment upload 42 --file build.log --description "CI output"
```

## opcli wp backfill

Record work done after the fact: real dates, hours per day and an audit comment

Register work that already happened without being planned or logged.

In one call it creates the work package (--subject) or reuses an existing one
(--wp), sets its start/finish dates to the real ones (--from/--to, manual
scheduling), spreads --hours over the working days of that range (or logs
--hours-per-day on each), optionally moves it to --status, and adds a comment
recording that it was registered retroactively.

The creation date (createdAt) cannot be changed through the API; reports and
the Gantt should use start/finish dates and the time entries' spent date.

Working days come from the instance calendar (weekends and holidays are
skipped); --include-weekends logs on every day. Hours are split in 15-minute
steps, the remainder going to the first days. Time is logged for --user, else
the assignee, else you. The audit comment follows the configured language
(language: es in the config for Spanish). Notifications are off by default. Use --dry-run to
see the plan without writing anything.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--wp` | string |  | Existing work package id (omit and pass --subject to create one) |
| `--subject` | string |  | Title of the new work package (or new title for --wp) |
| `--project` | string |  | Project of the new work package (default: configured project) |
| `--type` | string |  | Type of the new work package (default: config default_type, else the project's default) |
| `--description` | string |  | Description in Markdown |
| `--assignee` | string |  | Who did the work: me, login, email, name or id |
| `--parent` | string |  | Parent work package id |
| `--status` | string |  | Final status, e.g. Closed (applied after logging time) |
| `--from` | string |  | **required** First real day of work: YYYY-MM-DD, yesterday, -Nd |
| `--to` | string |  | Last real day of work (default: same as --from) |
| `--hours` | string |  | Total time to spread over the working days: 12, 7.5, 1h30m |
| `--hours-per-day` | string |  | Time to log on each working day instead of a total |
| `--estimate` | string |  | Estimated work of a new work package (default: the logged total) |
| `--activity` | string |  | Time entry activity name or id (Development, Management, ...) |
| `--user` | string |  | Log time on behalf of this user (default: the assignee; needs permission) |
| `--comment` | string |  | Comment on each time entry (default: the subject) |
| `--reason` | string |  | Why it is registered late; appended to the audit comment |
| `--include-weekends` | bool |  | Log time on every day of the range, not only working days |
| `--notify` | bool | `false` | Send email notifications for these changes |

```sh
  opcli wp backfill -p demo-project --subject "Hotfix MQTT broker" --assignee jane.doe \
    --from 2026-09-14 --to 2026-09-18 --hours 12h --activity Development --status Closed --dry-run
  opcli wp backfill --wp 42 --from 2026-09-21 --to 2026-09-23 --hours-per-day 2h \
    --reason "Done during the incident, not reported until today"
  opcli wp backfill --wp 42 --from -10d --to -8d     # only fix the dates, no time entries
```

## opcli wp comment

Add a comment to a work package

Add a Markdown comment. Local images (![alt](shot.png)) are uploaded as
attachments of the work package and rendered inline; relative paths resolve
against --body-file (or the working directory). @login becomes a mention
that notifies the user (the result lists "mentioned", and
"unresolvedMentions" for what matched nobody). Fix a comment later with
'wp comment-edit'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--body` | string |  | **required** Comment text (Markdown) |
| `--internal` | bool |  | Internal comment, visible only to roles allowed to see internal comments (Enterprise; enabled per project) |

```sh
  opcli wp comment 42 --body "Deployed to staging"
  git log -1 --format=%B | opcli wp comment 42 --body-file -
  opcli wp comment 42 --body-file notes/review.md   # with ![](shot.png) inside
  opcli wp comment 42 --body "@jane can you review?"
  opcli wp comment 42 --body "Customer budget is tight" --internal
```

## opcli wp comment-edit

Replace the text of a comment

Replace the text of an existing comment. The id is the activity id shown
by 'wp activities' (or 'wp get --activities'). Only your own comments can be
edited unless your role allows editing others'. Comments cannot be deleted
through the API. Local images are handled as in 'wp comment'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Activity (comment) id |
| `--body` | string |  | **required** New comment text (Markdown) |

```sh
  opcli wp comment-edit 1234 --body "Deployed to production"
  opcli wp comment-edit 1234 --body-file fixed.md
```

## opcli wp create

Create a work package

Create a work package. Only --subject is required: the project falls back to
the configured default and the type to default_type from the config, else
the project's default type. Long descriptions are easier
to pass with --description-file (use - for stdin).

Custom fields and any other field go by name with --field NAME=VALUE (see
'opcli wp fields' for the names and allowed values). Before anything is
written the server validates the whole request through its form, so a
missing required field or a value that is not allowed fails with the list
of valid values, and --dry-run shows exactly what would be sent.

Local images in the description (![alt](shot.png)) are uploaded as
attachments and the link is rewritten so they render; relative paths resolve
against the --description-file (or the working directory). Remote URLs are
left alone. The files are claimed by the new work package in the same
request. @login in the description becomes a mention that notifies the user.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name (default: configured project) |
| `--subject` | string |  | **required** Title of the work package |
| `--attach` | list |  | Local file to attach (repeatable); a file already attached with the same name and size is not uploaded again |
| `--type` | string |  | Type name or id (Task, Bug, Feature, Epic, Milestone...); default: config default_type, else the project's default |
| `--description` | string |  | Description in Markdown |
| `--status` | string |  | Status name or id (see 'opcli wp allowed <id>') |
| `--priority` | string |  | Priority name or id |
| `--assignee` | string |  | me, none, or login/email/name/id |
| `--responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--parent` | string |  | Parent work package id ('none' to detach) |
| `--version` | string |  | Target version name or id ('none' to clear) |
| `--category` | string |  | Category name or id ('none' to clear) |
| `--start-date` | string |  | Start date: YYYY-MM-DD, today, tomorrow, +3d... ('none' to clear) |
| `--due-date` | string |  | Finish date: YYYY-MM-DD, today, +7d... ('none' to clear) |
| `--estimate` | string |  | Estimated work: 4, 1.5, 90m, 2h30m or PT2H ('none' to clear) |
| `--remaining` | string |  | Remaining work, same formats as --estimate |
| `--percent-done` | int |  | % complete (0-100), when progress is work-based |
| `--field` | list |  | Any field by its name, NAME=VALUE (repeatable): custom fields such as "Customer=ACME"; lists, users and versions by name; several values comma-separated; "NAME=" clears (see 'opcli wp fields') |
| `--fields` | string |  | Raw JSON object merged into the request, e.g. {"customField3":"x"}; prefer --field |
| `--notify` | bool | `true` | Send email notifications for this change |

```sh
  opcli wp create --subject "Fix login timeout" --type Bug --priority High
  opcli wp create -p demo-project --subject "Write docs" --assignee me --due-date +7d
  opcli wp create --subject "Sub-task" --parent 42 --estimate 3h
  opcli wp create --subject "Onboarding" --field "Customer=ACME" --field "Area=Backend"
  opcli wp create --subject "Crash on save" --type Bug --description-file bug.md --attach crash.log
  cat spec.md | opcli wp create --subject "Epic: SSO" --type Epic --description-file -
```

## opcli wp delete

Delete a work package (irreversible, children included)

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |

```sh
  opcli wp delete 42
```

## opcli wp fields

List the fields a work package takes: names, types, required, allowed values

List the writable fields of a work package as the server's form describes
them, custom fields included: the name to use with --field, the type, whether
it is required, the allowed values when they are a finite list, and the help
text configured in the instance.

With an id it describes that work package (statuses are the transitions the
workflow allows from its current status); without one it describes a new
work package of --type in --project.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | Work package id (omit to describe a new one) |
| `--project` | string |  | Project of the new work package (default: configured project) |
| `--type` | string |  | Type of the new work package (default: as in wp create) |
| `--custom` | bool |  | Only custom fields |

```sh
  opcli wp fields --project demo-project --type Task
  opcli wp fields 42
  opcli wp fields 42 --custom
  opcli wp fields -p demo-project -o json | jq -r '.items[] | select(.required) | .name'
```

## opcli wp get

Show a work package with description, relations, children and attachments

Show one work package: all its fields, the Markdown description, relations,
children, attachments and (with --activities) the comment/change history.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--activities` | bool |  | Also include comments and change history |

```sh
  opcli wp get 42
  opcli wp get 42 --activities
  opcli wp get 42 -o json
```

## opcli wp list

List and search work packages with filters

List work packages. Filters combine with AND. By default only open work
packages are shown, in the configured default project (if any), most
recently updated first.

--status accepts: open (default), closed, all, or a comma-separated list of
status names/ids ("New,In progress"). --assignee/--author/--responsible accept
me, none, a login, email, name or id. --search does full-text search over
subject, description and comments.

--field filters by any field by its name, custom fields included
("Customer=ACME"; text fields match by "contains", lists, users and versions
by name, "NAME=" finds empty ones). It needs a project. Custom fields come
back by name under "customFields".

--filter takes raw API filters (JSON) for anything not covered by flags, e.g.
  [{"dueDate":{"operator":"<t+","values":["7"]}}]
See 'opcli help filters' for operators. --query-id runs a saved query instead.

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `--project` | string |  | Project id, identifier or name; 'all' for every project (default: configured project) |
| `--status` | string | `open` | open, closed, all, or comma-separated status names/ids |
| `--type` | string |  | Comma-separated type names/ids (Task, Bug, Feature...) |
| `--assignee` | string |  | me, none, or login/email/name/id |
| `--author` | string |  | me, or login/email/name/id |
| `--responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--priority` | string |  | Comma-separated priority names/ids |
| `--version` | string |  | Target version name or id (name needs a project) |
| `--parent` | string |  | Only direct children of this work package id |
| `--search` | string |  | Full-text search in subject, description and comments |
| `--field` | list |  | Filter by any field by name, NAME=VALUE (repeatable): "Customer=ACME", "Area=Backend,Ops"; "NAME=" means empty; needs a project |
| `--filter` | string |  | Extra raw API filters as a JSON array (see 'opcli help filters') |
| `--sort` | string | `updatedAt:desc` | Sort spec field:asc\|desc, comma-separated (id, subject, updatedAt, dueDate, priority, status...) |
| `--query-id` | int |  | Run a saved query by id (other filters are ignored) |
| `--max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli wp list                                  # open WPs, default project
  opcli wp list --assignee me --project all      # my open work, everywhere
  opcli wp list -p demo-project --type Bug --status all
  opcli wp list --search "login timeout" --max 10
  opcli wp list --status "In progress,On hold" --sort dueDate:asc
  opcli wp list -p demo-project --field "Customer=ACME" --field "Billable=yes"
  opcli wp list --filter '[{"dueDate":{"operator":"<t+","values":["7"]}}]'
  opcli wp list --query-id 12                    # run a saved query
  opcli wp list --assignee me -o json            # machine-readable
```

## opcli wp relation add

Relate two work packages

Create a relation "<id> <type> <to>". For example "12 follows 10" means #12
starts after #10 finishes; "12 blocks 10" means #10 cannot be closed before
#12. For parent/child use 'opcli wp update <child> --parent <id>'.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package (from) id |
| `--to` | string |  | **required** Related work package id |
| `--type` | string | `relates` | Relation type: relates, duplicates, duplicated, blocks, blocked, precedes, follows, includes, partof, requires, required |
| `--lag` | int |  | Working days between the two (follows/precedes only) |
| `--description` | string |  | Optional description |

```sh
  opcli wp relation add 12 --type follows --to 10
  opcli wp relation add 12 --type relates --to 30 --description "same root cause"
  opcli wp relation add 12 --type precedes --to 13 --lag 2
```

## opcli wp relation delete

Delete a relation by its id (see 'wp relation list')

Effect: **destructive**

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Relation id |

```sh
  opcli wp relation delete 7
```

## opcli wp relation list

List relations of a work package

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |

```sh
  opcli wp relation list 42
```

## opcli wp remind

Remind yourself about a work package at a given time

Set a personal reminder: at that time OpenProject notifies you (in-app
and by email, as your notification settings say). Only you see it.

You can have one active reminder per work package: setting another one
moves the existing reminder to the new time (and note, if given). A bare
date means 09:00 that day; times are in your OpenProject time zone.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--at` | string |  | **required** When: "YYYY-MM-DD HH:MM", "tomorrow 9:00", "16:00", "+2h", a date (09:00) |
| `--note` | string |  | Why (shown in the notification) |

```sh
  opcli wp remind 42 --at "tomorrow 9:00" --note "Call the customer"
  opcli wp remind 42 --at "2026-10-20 16:30"
  opcli wp remind 42 --at +2h
```

## opcli wp update

Update fields of a work package (status, assignee, dates, ...)

Update only the fields you pass. The lock version is handled for you (a
concurrent edit triggers one automatic retry). Pass 'none' to clear user,
parent, version, category, dates or estimates. --comment adds a comment in
the same call. --project moves the work package to another project.

Custom fields go by name with --field NAME=VALUE ("NAME=" clears). The
server validates the change through its form before anything is written: a
status the workflow does not allow from the current one fails listing the
statuses that are, and so does any other value that is not allowed.

Local images in --description or --comment, and --attach files, are
uploaded after that check (see 'wp create'); a file already attached with
the same name and size is reused, so re-running the same update attaches
nothing new. @login in the text becomes a mention that notifies the user.

Several ids apply the same change to each, one after the other; each one
is validated on its own, and the error names the ones that failed and the
ones that were updated. Run it with --dry-run first.

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id, or several (42 43 44) to apply the same change to each |
| `--subject` | string |  | New title |
| `--project` | string |  | Move to this project (id, identifier or name) |
| `--comment` | string |  | Also add this comment (Markdown) |
| `--internal` | bool |  | Internal comment, visible only to roles allowed to see internal comments (Enterprise; enabled per project) |
| `--attach` | list |  | Local file to attach (repeatable); a file already attached with the same name and size is not uploaded again |
| `--type` | string |  | Type name or id (Task, Bug, Feature, Epic, Milestone...); default: config default_type, else the project's default |
| `--description` | string |  | Description in Markdown |
| `--status` | string |  | Status name or id (see 'opcli wp allowed <id>') |
| `--priority` | string |  | Priority name or id |
| `--assignee` | string |  | me, none, or login/email/name/id |
| `--responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--parent` | string |  | Parent work package id ('none' to detach) |
| `--version` | string |  | Target version name or id ('none' to clear) |
| `--category` | string |  | Category name or id ('none' to clear) |
| `--start-date` | string |  | Start date: YYYY-MM-DD, today, tomorrow, +3d... ('none' to clear) |
| `--due-date` | string |  | Finish date: YYYY-MM-DD, today, +7d... ('none' to clear) |
| `--estimate` | string |  | Estimated work: 4, 1.5, 90m, 2h30m or PT2H ('none' to clear) |
| `--remaining` | string |  | Remaining work, same formats as --estimate |
| `--percent-done` | int |  | % complete (0-100), when progress is work-based |
| `--field` | list |  | Any field by its name, NAME=VALUE (repeatable): custom fields such as "Customer=ACME"; lists, users and versions by name; several values comma-separated; "NAME=" clears (see 'opcli wp fields') |
| `--fields` | string |  | Raw JSON object merged into the request, e.g. {"customField3":"x"}; prefer --field |
| `--notify` | bool | `true` | Send email notifications for this change |

```sh
  opcli wp update 42 --status "In progress" --assignee me
  opcli wp update 42 43 44 --version "Sprint 7" --dry-run
  opcli wp update 42 --status Closed --comment "Fixed in v1.2"
  opcli wp update 42 --comment-file review.md --attach report.pdf
  opcli wp update 42 --due-date 2026-10-15 --estimate 6h
  opcli wp update 42 --assignee none --version none
  opcli wp update 42 --field "Customer=ACME" --field "Tags=urgent,customer"
  opcli wp update 42 --field "Customer="          # clear a custom field
```

## opcli wp watcher add

Add a watcher to a work package

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--user` | string |  | **required** me, or login/email/name/id |

```sh
  opcli wp watcher add 42 --user me
  opcli wp watcher add 42 --user jane.doe
```

## opcli wp watcher list

List watchers of a work package

Effect: read-only

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |

```sh
  opcli wp watcher list 42
```

## opcli wp watcher remove

Remove a watcher from a work package

Effect: writes

| Flag | Type | Default | Description |
|---|---|---|---|
| `<id>` (arg) | string |  | **required** Work package id |
| `--user` | string |  | **required** me, or login/email/name/id |

```sh
  opcli wp watcher remove 42 --user me
```
