# opcli command reference

Generated with `opcli docs`. Every command is also an MCP tool with the same parameters (flags in kebab-case ↔ tool parameters in snake_case). Global flags: `--config`, `--url`, `--api-key`, `-o text|json`.

| Command | MCP tool | Effect | Summary |
|---|---|---|---|
| `opcli api describe` | `op_api_describe_endpoint` | read-only | Show parameters, body fields and examples of an endpoint |
| `opcli api endpoints` | `op_api_list_endpoints` | read-only | Search the catalogue of API v3 endpoints |
| `opcli api request` | `op_api_request` | **destructive** | Call any API v3 endpoint and print the JSON answer |
| `opcli meeting get` | `op_get_meeting` | read-only | Show a meeting with its agenda items |
| `opcli meeting list` | `op_list_meetings` | read-only | List meetings |
| `opcli membership add` | `op_create_membership` | writes | Add a user or group to a project with one or more roles |
| `opcli membership delete` | `op_delete_membership` | **destructive** | Remove a membership (the user loses access to the project) |
| `opcli membership list` | `op_list_memberships` | read-only | List memberships, optionally by project and/or user |
| `opcli membership update` | `op_update_membership` | writes | Replace the roles of a membership |
| `opcli notification list` | `op_list_notifications` | read-only | List your notifications (unread by default) |
| `opcli notification read` | `op_mark_notifications_read` | writes | Mark one notification (by id) or all unread notifications as read |
| `opcli priority list` | `op_list_priorities` | read-only | List work package priorities |
| `opcli project assignees` | `op_list_project_assignees` | read-only | List users that can be assigned work packages in a project |
| `opcli project categories` | `op_list_project_categories` | read-only | List work package categories of a project |
| `opcli project create` | `op_create_project` | writes | Create a project |
| `opcli project delete` | `op_delete_project` | **destructive** | Delete a project and everything in it (irreversible) |
| `opcli project get` | `op_get_project` | read-only | Show a project |
| `opcli project list` | `op_list_projects` | read-only | List projects |
| `opcli project update` | `op_update_project` | writes | Update a project (name, description, status, archive...) |
| `opcli query list` | `op_list_queries` | read-only | List saved work package queries; run one with 'opcli wp list --query-id <id>' |
| `opcli role list` | `op_list_roles` | read-only | List roles |
| `opcli status list` | `op_list_statuses` | read-only | List all work package statuses |
| `opcli time activities` | `op_list_time_activities` | read-only | List activities valid for time entries (optionally for a work package) |
| `opcli time delete` | `op_delete_time_entry` | **destructive** | Delete a time entry |
| `opcli time list` | `op_list_time_entries` | read-only | List time entries (yours by default) with an hour total |
| `opcli time log` | `op_log_time` | writes | Log time on a work package |
| `opcli time update` | `op_update_time_entry` | writes | Update a time entry |
| `opcli type list` | `op_list_types` | read-only | List work package types, globally or enabled in a project |
| `opcli user get` | `op_get_user` | read-only | Show a user |
| `opcli user list` | `op_list_users` | read-only | List or search users, groups and placeholder users |
| `opcli version create` | `op_create_version` | writes | Create a version in a project |
| `opcli version list` | `op_list_versions` | read-only | List versions of a project (or all visible versions with --project all) |
| `opcli version update` | `op_update_version` | writes | Update a version (dates, status, name...) |
| `opcli whoami` | `op_whoami` | read-only | Show the authenticated user (checks URL and API key) |
| `opcli wp activities` | `op_list_work_package_activities` | read-only | Show comments and change history of a work package |
| `opcli wp allowed` | `op_list_work_package_allowed_values` | read-only | Show values a work package accepts: statuses, types, priorities, versions... |
| `opcli wp attachment delete` | `op_delete_attachment` | **destructive** | Delete an attachment by its id |
| `opcli wp attachment download` | `op_download_attachment` | read-only | Download an attachment by its id |
| `opcli wp attachment list` | `op_list_attachments` | read-only | List attachments of a work package |
| `opcli wp attachment upload` | `op_upload_attachment` | writes | Attach a local file to a work package |
| `opcli wp backfill` | `op_backfill_work_package` | writes | Record work done after the fact: real dates, hours per day and an audit comment |
| `opcli wp comment` | `op_comment_work_package` | writes | Add a comment to a work package |
| `opcli wp create` | `op_create_work_package` | writes | Create a work package |
| `opcli wp delete` | `op_delete_work_package` | **destructive** | Delete a work package (irreversible, children included) |
| `opcli wp get` | `op_get_work_package` | read-only | Show a work package with description, relations, children and attachments |
| `opcli wp list` | `op_list_work_packages` | read-only | List and search work packages with filters |
| `opcli wp relation add` | `op_create_relation` | writes | Relate two work packages |
| `opcli wp relation delete` | `op_delete_relation` | **destructive** | Delete a relation by its id (see 'wp relation list') |
| `opcli wp relation list` | `op_list_relations` | read-only | List relations of a work package |
| `opcli wp update` | `op_update_work_package` | writes | Update fields of a work package (status, assignee, dates, ...) |
| `opcli wp watcher add` | `op_add_watcher` | writes | Add a watcher to a work package |
| `opcli wp watcher list` | `op_list_watchers` | read-only | List watchers of a work package |
| `opcli wp watcher remove` | `op_remove_watcher` | writes | Remove a watcher from a work package |

## opcli api describe

Show parameters, body fields and examples of an endpoint

Describe an endpoint from the OpenAPI spec. The path can be the template
(/api/v3/work_packages/{id}) or a concrete path (/api/v3/work_packages/42).
Omit the method to describe every method of the path.

MCP tool: `op_api_describe_endpoint` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<method-or-path>` (arg) | `method_or_path` | string |  | **required** HTTP method, or the path when describing all methods |
| `<path>` (arg) | `path` | string |  | Endpoint path (when a method is given first) |
| `--live` | `live` | bool |  | Use the spec served by the configured instance instead of the embedded one |

```sh
  opcli api describe /api/v3/time_entries
  opcli api describe POST /api/v3/time_entries
  opcli api describe PATCH work_packages/42 -o json
```

## opcli api endpoints

Search the catalogue of API v3 endpoints

List endpoints whose method, path, tag or summary contain every given word.

MCP tool: `op_api_list_endpoints` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<search>` (arg) | `search` | string |  | Words to match (all must match) |
| `--live` | `live` | bool |  | Use the spec served by the configured instance instead of the embedded one |

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

In read-only MCP mode only GET is allowed.

MCP tool: `op_api_request` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<method>` (arg) | `method` | string |  | **required** HTTP method |
| `<path>` (arg) | `path` | string |  | **required** Endpoint path, e.g. /api/v3/projects or projects/3 |
| `--query` | `query` | list |  | Query parameter key=value (repeatable) |
| `--body` | `body` | string |  | JSON request body |
| `--flatten` | `flatten` | bool |  | Simplify HAL output |

```sh
  opcli api request GET /api/v3/configuration
  opcli api request GET work_packages --query 'filters=[{"status":{"operator":"o","values":[]}}]' --query pageSize=5 --flatten
  opcli api request POST /api/v3/render/markdown --body '"**bold**"'
  opcli api request PATCH work_packages/42 --body '{"lockVersion":3,"subject":"New"}'
  opcli api request POST news --body-file news.json
```

## opcli meeting get

Show a meeting with its agenda items

MCP tool: `op_get_meeting` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Meeting id |

```sh
  opcli meeting get 3
```

## opcli meeting list

List meetings

MCP tool: `op_list_meetings` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Only meetings of this project (default: all visible) |
| `--max` | `max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | `page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli meeting list
  opcli meeting list -p demo-project
```

## opcli membership add

Add a user or group to a project with one or more roles

MCP tool: `op_create_membership` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name (default: configured project) |
| `--user` | `user` | string |  | **required** User/group: login, email, name or id |
| `--roles` | `roles` | list |  | **required** Role names or ids (see 'opcli role list') |
| `--message` | `message` | string |  | Custom text for the notification email |

```sh
  opcli membership add -p demo-project --user jane.doe --roles Member
  opcli membership add -p demo-project --user devs --roles "Member,Reader" --message "Welcome!"
```

## opcli membership delete

Remove a membership (the user loses access to the project)

MCP tool: `op_delete_membership` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Membership id |

```sh
  opcli membership delete 12
```

## opcli membership list

List memberships, optionally by project and/or user

MCP tool: `op_list_memberships` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name (default: all projects) |
| `--user` | `user` | string |  | me, or login/email/name/id |
| `--max` | `max` | int | `200` | Maximum number of results (0 = all) |

```sh
  opcli membership list -p demo-project
  opcli membership list --user me
```

## opcli membership update

Replace the roles of a membership

MCP tool: `op_update_membership` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Membership id |
| `--roles` | `roles` | list |  | **required** Role names or ids |

```sh
  opcli membership update 12 --roles "Project admin"
```

## opcli notification list

List your notifications (unread by default)

MCP tool: `op_list_notifications` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--all` | `all` | bool |  | Include already read notifications |
| `--reason` | `reason` | string |  | Only this reason: mentioned, assigned, responsible, watched, commented, created, processed, prioritized, scheduled, dateAlert, shared |
| `--project` | `project` | string |  | Only notifications of this project |
| `--max` | `max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | `page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli notification list
  opcli notification list --all --max 100
  opcli notification list --reason mentioned
```

## opcli notification read

Mark one notification (by id) or all unread notifications as read

MCP tool: `op_mark_notifications_read` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | Notification id (omit with --all) |
| `--all` | `all` | bool |  | Mark every unread notification as read |

```sh
  opcli notification read 123
  opcli notification read --all
```

## opcli priority list

List work package priorities

MCP tool: `op_list_priorities` · read-only

```sh
  opcli priority list
```

## opcli project assignees

List users that can be assigned work packages in a project

MCP tool: `op_list_project_assignees` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<project>` (arg) | `project` | string |  | Project id, identifier or name (default: configured project) |

```sh
  opcli project assignees demo-project
```

## opcli project categories

List work package categories of a project

MCP tool: `op_list_project_categories` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<project>` (arg) | `project` | string |  | Project id, identifier or name (default: configured project) |

```sh
  opcli project categories demo-project
```

## opcli project create

Create a project

Create a project. The identifier is derived from the name when omitted.

MCP tool: `op_create_project` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--name` | `name` | string |  | **required** Project name |
| `--identifier` | `identifier` | string |  | URL identifier (lowercase, digits, dashes) |
| `--description` | `description` | string |  | Description (Markdown) |
| `--parent` | `parent` | string |  | Parent project id/identifier/name ('none' for top level) |
| `--public` | `public` | bool |  | Visible to everybody |
| `--status` | `status` | string |  | Project status: not_started, on_track, at_risk, off_track, finished, discontinued |
| `--status-explanation` | `status_explanation` | string |  | Status explanation (Markdown) |

```sh
  opcli project create --name "Website relaunch"
  opcli project create --name "Backend" --identifier web-backend --parent website-relaunch --public
```

## opcli project delete

Delete a project and everything in it (irreversible)

MCP tool: `op_delete_project` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<project>` (arg) | `project` | string |  | **required** Project id, identifier or name |

```sh
  opcli project delete old-stuff
```

## opcli project get

Show a project

MCP tool: `op_get_project` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<project>` (arg) | `project` | string |  | **required** Project id, identifier or name |

```sh
  opcli project get demo-project
  opcli project get 3 -o json
```

## opcli project list

List projects

MCP tool: `op_list_projects` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--search` | `search` | string |  | Filter by name or identifier (substring) |
| `--archived` | `archived` | bool |  | List archived projects instead of active ones |
| `--filter` | `filter` | string |  | Extra raw API filters as a JSON array |
| `--max` | `max` | int | `100` | Maximum number of results (0 = all) |
| `--page` | `page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli project list
  opcli project list --search radar
  opcli project list --archived
```

## opcli project update

Update a project (name, description, status, archive...)

MCP tool: `op_update_project` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<project>` (arg) | `project` | string |  | **required** Project id, identifier or name |
| `--name` | `name` | string |  | New name |
| `--identifier` | `identifier` | string |  | New identifier |
| `--archive` | `archive` | bool |  | Archive the project (true) or unarchive it (false) |
| `--description` | `description` | string |  | Description (Markdown) |
| `--parent` | `parent` | string |  | Parent project id/identifier/name ('none' for top level) |
| `--public` | `public` | bool |  | Visible to everybody |
| `--status` | `status` | string |  | Project status: not_started, on_track, at_risk, off_track, finished, discontinued |
| `--status-explanation` | `status_explanation` | string |  | Status explanation (Markdown) |

```sh
  opcli project update demo-project --status at_risk --status-explanation "Vendor late"
  opcli project update 3 --name "New name"
  opcli project update old-stuff --archive
```

## opcli query list

List saved work package queries; run one with 'opcli wp list --query-id <id>'

MCP tool: `op_list_queries` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Only queries of this project (default: all visible) |

```sh
  opcli query list
  opcli query list -p demo-project
  opcli wp list --query-id 5
```

## opcli role list

List roles

MCP tool: `op_list_roles` · read-only

```sh
  opcli role list
```

## opcli status list

List all work package statuses

List every status. Which ones a given work package may move to depends on the workflow: use 'opcli wp allowed <id> --field status'.

MCP tool: `op_list_statuses` · read-only

```sh
  opcli status list
```

## opcli time activities

List activities valid for time entries (optionally for a work package)

MCP tool: `op_list_time_activities` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--wp` | `wp` | string |  | Work package id (activities can be restricted per project) |

```sh
  opcli time activities
  opcli time activities --wp 42
```

## opcli time delete

Delete a time entry

MCP tool: `op_delete_time_entry` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Time entry id |

```sh
  opcli time delete 17
```

## opcli time list

List time entries (yours by default) with an hour total

MCP tool: `op_list_time_entries` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--user` | `user` | string | `me` | me (default), all, or login/email/name/id |
| `--project` | `project` | string |  | Project id, identifier or name (default: all projects) |
| `--wp` | `wp` | string |  | Only entries of this work package id |
| `--from` | `from` | string | `week` | First day (inclusive): YYYY-MM-DD, today, yesterday, week, month, -Nd |
| `--to` | `to` | string | `today` | Last day (inclusive) |
| `--group-by` | `group_by` | string |  | Summarise hours by day, project, wp, activity or user instead of listing entries |
| `--max` | `max` | int | `500` | Maximum number of results (0 = all) |

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

MCP tool: `op_log_time` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--wp` | `wp` | string |  | **required** Work package id |
| `--hours` | `hours` | string |  | **required** Time spent: 1.5, 90m, 1h30m, PT1H30M |
| `--date` | `date` | string | `today` | Day the work was done (YYYY-MM-DD, today, yesterday, -Nd) |
| `--comment` | `comment` | string |  | What was done |
| `--activity` | `activity` | string |  | Activity name or id (Development, Management, ...) |
| `--user` | `user` | string |  | Log on behalf of this user (needs permission) |

```sh
  opcli time log --wp 42 --hours 1.5 --comment "Code review"
  opcli time log --wp 42 --hours 2h --date yesterday --activity Development
  opcli time log --wp 42 --hours 45m --user jane.doe      # admins: on behalf of others
```

## opcli time update

Update a time entry

MCP tool: `op_update_time_entry` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Time entry id |
| `--wp` | `wp` | string |  | Move to this work package id |
| `--hours` | `hours` | string |  | Time spent: 1.5, 90m, 1h30m, PT1H30M |
| `--date` | `date` | string |  | Day the work was done |
| `--comment` | `comment` | string |  | What was done |
| `--activity` | `activity` | string |  | Activity name or id |

```sh
  opcli time update 17 --hours 3 --comment "Longer than expected"
  opcli time update 17 --date 2026-09-24 --wp 43
```

## opcli type list

List work package types, globally or enabled in a project

MCP tool: `op_list_types` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Only types enabled in this project (default: all types) |

```sh
  opcli type list
  opcli type list -p demo-project
```

## opcli user get

Show a user

MCP tool: `op_get_user` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<user>` (arg) | `user` | string |  | **required** me, or login/email/name/id |

```sh
  opcli user get me
  opcli user get jane.doe
  opcli user get 8 -o json
```

## opcli user list

List or search users, groups and placeholder users

MCP tool: `op_list_users` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--search` | `search` | string |  | Match name, login or email (substring) |
| `--kind` | `kind` | string | `user` | user (default), group, placeholder or all |
| `--project` | `project` | string |  | Only members of this project |
| `--max` | `max` | int | `100` | Maximum number of results (0 = all) |
| `--page` | `page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli user list
  opcli user list --search jane
  opcli user list --kind group
  opcli user list -p demo-project           # members of a project
```

## opcli version create

Create a version in a project

MCP tool: `op_create_version` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name (default: configured project) |
| `--name` | `name` | string |  | **required** Version name |
| `--description` | `description` | string |  | Description |
| `--start-date` | `start_date` | string |  | Start date (YYYY-MM-DD, today, +7d...) |
| `--end-date` | `end_date` | string |  | Finish date (YYYY-MM-DD, +14d...) |
| `--status` | `status` | string |  | open, locked or closed |
| `--sharing` | `sharing` | string |  | Share with: none, descendants, hierarchy, tree, system |

```sh
  opcli version create -p demo-project --name "v1.2" --end-date 2026-11-30
```

## opcli version list

List versions of a project (or all visible versions with --project all)

MCP tool: `op_list_versions` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name; 'all' for every project |

```sh
  opcli version list -p demo-project
  opcli version list --project all
```

## opcli version update

Update a version (dates, status, name...)

MCP tool: `op_update_version` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Version id |
| `--name` | `name` | string |  | New name |
| `--description` | `description` | string |  | Description |
| `--start-date` | `start_date` | string |  | Start date (YYYY-MM-DD, today, +7d...) |
| `--end-date` | `end_date` | string |  | Finish date (YYYY-MM-DD, +14d...) |
| `--status` | `status` | string |  | open, locked or closed |
| `--sharing` | `sharing` | string |  | Share with: none, descendants, hierarchy, tree, system |

```sh
  opcli version update 4 --status closed
```

## opcli whoami

Show the authenticated user (checks URL and API key)

MCP tool: `op_whoami` · read-only

```sh
  opcli whoami
  opcli whoami -o json
```

## opcli wp activities

Show comments and change history of a work package

MCP tool: `op_list_work_package_activities` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--comments-only` | `comments_only` | bool |  | Only entries with a comment |

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

MCP tool: `op_list_work_package_allowed_values` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--field` | `field` | string |  | Only this field (status, type, priority, category, version, assignee, responsible, project, projectPhase) |

```sh
  opcli wp allowed 42 --field status
  opcli wp allowed 42 --field assignee
  opcli wp allowed 42 -o json
```

## opcli wp attachment delete

Delete an attachment by its id

MCP tool: `op_delete_attachment` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Attachment id |

```sh
  opcli wp attachment delete 15
```

## opcli wp attachment download

Download an attachment by its id

Download an attachment (ids come from 'wp attachment list' or 'wp get').
The file is written to --dest (default: its original name in the current
directory). With --stdout text content is returned instead of saved, which
is how an AI agent can read a text attachment.

MCP tool: `op_download_attachment` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Attachment id |
| `--dest` | `dest` | string |  | Destination path (default: original file name) |
| `--stdout` | `stdout` | bool |  | Return the content instead of writing a file (text files only) |

```sh
  opcli wp attachment download 15
  opcli wp attachment download 15 --dest /tmp/log.txt
  opcli wp attachment download 15 --stdout | less
```

## opcli wp attachment list

List attachments of a work package

MCP tool: `op_list_attachments` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |

```sh
  opcli wp attachment list 42
```

## opcli wp attachment upload

Attach a local file to a work package

Upload a file. With MCP the path is read on the machine running the
opcli MCP server, not on the client.

MCP tool: `op_upload_attachment` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--file` | `file` | string |  | **required** Path of the file to upload |
| `--description` | `description` | string |  | Optional description |

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

MCP tool: `op_backfill_work_package` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--wp` | `wp` | string |  | Existing work package id (omit and pass --subject to create one) |
| `--subject` | `subject` | string |  | Title of the new work package (or new title for --wp) |
| `--project` | `project` | string |  | Project of the new work package (default: configured project) |
| `--type` | `type` | string |  | Type of the new work package (default: config default_type, else the project's default) |
| `--description` | `description` | string |  | Description in Markdown |
| `--assignee` | `assignee` | string |  | Who did the work: me, login, email, name or id |
| `--parent` | `parent` | string |  | Parent work package id |
| `--status` | `status` | string |  | Final status, e.g. Closed (applied after logging time) |
| `--from` | `from` | string |  | **required** First real day of work: YYYY-MM-DD, yesterday, -Nd |
| `--to` | `to` | string |  | Last real day of work (default: same as --from) |
| `--hours` | `hours` | string |  | Total time to spread over the working days: 12, 7.5, 1h30m |
| `--hours-per-day` | `hours_per_day` | string |  | Time to log on each working day instead of a total |
| `--estimate` | `estimate` | string |  | Estimated work of a new work package (default: the logged total) |
| `--activity` | `activity` | string |  | Time entry activity name or id (Development, Management, ...) |
| `--user` | `user` | string |  | Log time on behalf of this user (default: the assignee; needs permission) |
| `--comment` | `comment` | string |  | Comment on each time entry (default: the subject) |
| `--reason` | `reason` | string |  | Why it is registered late; appended to the audit comment |
| `--include-weekends` | `include_weekends` | bool |  | Log time on every day of the range, not only working days |
| `--dry-run` | `dry_run` | bool |  | Show the plan without writing anything |
| `--notify` | `notify` | bool | `false` | Send email notifications for these changes |

```sh
  opcli wp backfill -p demo-project --subject "Hotfix MQTT broker" --assignee jane.doe \
    --from 2026-09-14 --to 2026-09-18 --hours 12h --activity Development --status Closed --dry-run
  opcli wp backfill --wp 42 --from 2026-09-21 --to 2026-09-23 --hours-per-day 2h \
    --reason "Done during the incident, not reported until today"
  opcli wp backfill --wp 42 --from -10d --to -8d     # only fix the dates, no time entries
```

## opcli wp comment

Add a comment to a work package

Add a Markdown comment. Mention users with the OpenProject syntax if needed.

MCP tool: `op_comment_work_package` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--body` | `body` | string |  | **required** Comment text (Markdown) |

```sh
  opcli wp comment 42 --body "Deployed to staging"
  git log -1 --format=%B | opcli wp comment 42 --body-file -
```

## opcli wp create

Create a work package

Create a work package. Only --subject is required: the project falls back to
the configured default and the type to default_type from the config, else
the project's default type. Long descriptions are easier
to pass with --description-file (use - for stdin).

MCP tool: `op_create_work_package` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name (default: configured project) |
| `--subject` | `subject` | string |  | **required** Title of the work package |
| `--type` | `type` | string |  | Type name or id (Task, Bug, Feature, Epic, Milestone...); default: config default_type, else the project's default |
| `--description` | `description` | string |  | Description in Markdown |
| `--status` | `status` | string |  | Status name or id (see 'opcli wp allowed <id>') |
| `--priority` | `priority` | string |  | Priority name or id |
| `--assignee` | `assignee` | string |  | me, none, or login/email/name/id |
| `--responsible` | `responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--parent` | `parent` | string |  | Parent work package id ('none' to detach) |
| `--version` | `version` | string |  | Target version name or id ('none' to clear) |
| `--category` | `category` | string |  | Category name or id ('none' to clear) |
| `--start-date` | `start_date` | string |  | Start date: YYYY-MM-DD, today, tomorrow, +3d... ('none' to clear) |
| `--due-date` | `due_date` | string |  | Finish date: YYYY-MM-DD, today, +7d... ('none' to clear) |
| `--estimate` | `estimate` | string |  | Estimated work: 4, 1.5, 90m, 2h30m or PT2H ('none' to clear) |
| `--remaining` | `remaining` | string |  | Remaining work, same formats as --estimate |
| `--percent-done` | `percent_done` | int |  | % complete (0-100), when progress is work-based |
| `--fields` | `fields` | string |  | Raw JSON object merged into the request, for custom fields or anything else, e.g. {"customField3":"x"} |
| `--notify` | `notify` | bool | `true` | Send email notifications for this change |

```sh
  opcli wp create --subject "Fix login timeout" --type Bug --priority High
  opcli wp create -p demo-project --subject "Write docs" --assignee me --due-date +7d
  opcli wp create --subject "Sub-task" --parent 42 --estimate 3h
  cat spec.md | opcli wp create --subject "Epic: SSO" --type Epic --description-file -
```

## opcli wp delete

Delete a work package (irreversible, children included)

MCP tool: `op_delete_work_package` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |

```sh
  opcli wp delete 42
```

## opcli wp get

Show a work package with description, relations, children and attachments

Show one work package: all its fields, the Markdown description, relations,
children, attachments and (with --activities) the comment/change history.

MCP tool: `op_get_work_package` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--activities` | `activities` | bool |  | Also include comments and change history |

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

--filter takes raw API filters (JSON) for anything not covered by flags, e.g.
  [{"dueDate":{"operator":"<t+","values":["7"]}}]
See 'opcli help filters' for operators. --query-id runs a saved query instead.

MCP tool: `op_list_work_packages` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `--project` | `project` | string |  | Project id, identifier or name; 'all' for every project (default: configured project) |
| `--status` | `status` | string | `open` | open, closed, all, or comma-separated status names/ids |
| `--type` | `type` | string |  | Comma-separated type names/ids (Task, Bug, Feature...) |
| `--assignee` | `assignee` | string |  | me, none, or login/email/name/id |
| `--author` | `author` | string |  | me, or login/email/name/id |
| `--responsible` | `responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--priority` | `priority` | string |  | Comma-separated priority names/ids |
| `--version` | `version` | string |  | Target version name or id (name needs a project) |
| `--parent` | `parent` | string |  | Only direct children of this work package id |
| `--search` | `search` | string |  | Full-text search in subject, description and comments |
| `--filter` | `filter` | string |  | Extra raw API filters as a JSON array (see 'opcli help filters') |
| `--sort` | `sort` | string | `updatedAt:desc` | Sort spec field:asc\|desc, comma-separated (id, subject, updatedAt, dueDate, priority, status...) |
| `--query-id` | `query_id` | int |  | Run a saved query by id (other filters are ignored) |
| `--max` | `max` | int | `50` | Maximum number of results (0 = all) |
| `--page` | `page` | int |  | Fetch only this page (1-based) of size --max instead of paging automatically |

```sh
  opcli wp list                                  # open WPs, default project
  opcli wp list --assignee me --project all      # my open work, everywhere
  opcli wp list -p demo-project --type Bug --status all
  opcli wp list --search "login timeout" --max 10
  opcli wp list --status "In progress,On hold" --sort dueDate:asc
  opcli wp list --filter '[{"dueDate":{"operator":"<t+","values":["7"]}}]'
  opcli wp list --query-id 12                    # run a saved query
  opcli wp list --assignee me -o json            # machine-readable
```

## opcli wp relation add

Relate two work packages

Create a relation "<id> <type> <to>". For example "12 follows 10" means #12
starts after #10 finishes; "12 blocks 10" means #10 cannot be closed before
#12. For parent/child use 'opcli wp update <child> --parent <id>'.

MCP tool: `op_create_relation` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package (from) id |
| `--to` | `to` | string |  | **required** Related work package id |
| `--type` | `type` | string | `relates` | Relation type: relates, duplicates, duplicated, blocks, blocked, precedes, follows, includes, partof, requires, required |
| `--lag` | `lag` | int |  | Working days between the two (follows/precedes only) |
| `--description` | `description` | string |  | Optional description |

```sh
  opcli wp relation add 12 --type follows --to 10
  opcli wp relation add 12 --type relates --to 30 --description "same root cause"
  opcli wp relation add 12 --type precedes --to 13 --lag 2
```

## opcli wp relation delete

Delete a relation by its id (see 'wp relation list')

MCP tool: `op_delete_relation` · **destructive**

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Relation id |

```sh
  opcli wp relation delete 7
```

## opcli wp relation list

List relations of a work package

MCP tool: `op_list_relations` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |

```sh
  opcli wp relation list 42
```

## opcli wp update

Update fields of a work package (status, assignee, dates, ...)

Update only the fields you pass. The lock version is handled for you (a
concurrent edit triggers one automatic retry). Pass 'none' to clear user,
parent, version, category, dates or estimates. --comment adds a comment in
the same call. --project moves the work package to another project.

MCP tool: `op_update_work_package` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--subject` | `subject` | string |  | New title |
| `--project` | `project` | string |  | Move to this project (id, identifier or name) |
| `--comment` | `comment` | string |  | Also add this comment (Markdown) |
| `--type` | `type` | string |  | Type name or id (Task, Bug, Feature, Epic, Milestone...); default: config default_type, else the project's default |
| `--description` | `description` | string |  | Description in Markdown |
| `--status` | `status` | string |  | Status name or id (see 'opcli wp allowed <id>') |
| `--priority` | `priority` | string |  | Priority name or id |
| `--assignee` | `assignee` | string |  | me, none, or login/email/name/id |
| `--responsible` | `responsible` | string |  | Accountable user: me, none, or login/email/name/id |
| `--parent` | `parent` | string |  | Parent work package id ('none' to detach) |
| `--version` | `version` | string |  | Target version name or id ('none' to clear) |
| `--category` | `category` | string |  | Category name or id ('none' to clear) |
| `--start-date` | `start_date` | string |  | Start date: YYYY-MM-DD, today, tomorrow, +3d... ('none' to clear) |
| `--due-date` | `due_date` | string |  | Finish date: YYYY-MM-DD, today, +7d... ('none' to clear) |
| `--estimate` | `estimate` | string |  | Estimated work: 4, 1.5, 90m, 2h30m or PT2H ('none' to clear) |
| `--remaining` | `remaining` | string |  | Remaining work, same formats as --estimate |
| `--percent-done` | `percent_done` | int |  | % complete (0-100), when progress is work-based |
| `--fields` | `fields` | string |  | Raw JSON object merged into the request, for custom fields or anything else, e.g. {"customField3":"x"} |
| `--notify` | `notify` | bool | `true` | Send email notifications for this change |

```sh
  opcli wp update 42 --status "In progress" --assignee me
  opcli wp update 42 --status Closed --comment "Fixed in v1.2"
  opcli wp update 42 --due-date 2026-10-15 --estimate 6h
  opcli wp update 42 --assignee none --version none
  opcli wp update 42 --fields '{"customField3":"ACME"}'
```

## opcli wp watcher add

Add a watcher to a work package

MCP tool: `op_add_watcher` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--user` | `user` | string |  | **required** me, or login/email/name/id |

```sh
  opcli wp watcher add 42 --user me
  opcli wp watcher add 42 --user jane.doe
```

## opcli wp watcher list

List watchers of a work package

MCP tool: `op_list_watchers` · read-only

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |

```sh
  opcli wp watcher list 42
```

## opcli wp watcher remove

Remove a watcher from a work package

MCP tool: `op_remove_watcher` · writes

| Flag | MCP param | Type | Default | Description |
|---|---|---|---|---|
| `<id>` (arg) | `id` | string |  | **required** Work package id |
| `--user` | `user` | string |  | **required** me, or login/email/name/id |

```sh
  opcli wp watcher remove 42 --user me
```
