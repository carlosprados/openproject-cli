package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() {
	registerGroup(&Group{
		Name:    "wp",
		Aliases: []string{"work-package", "workpackage", "task"},
		Short:   "Work packages: list, view, create, update, comment, relate, watch, attach",
		Long: `Work packages are OpenProject's tasks, bugs, features, epics, milestones...

References are resolved by name, so you rarely need ids:
  --status "In progress"   --type Bug   --priority High
  --assignee me|none|<login|email|name|id>   --project <id|identifier|name>

Use 'opcli wp allowed <id>' to see which statuses, types, priorities,
versions... a given work package accepts (workflows restrict transitions).`,
	})

	register(wpList, wpGet, wpCreate, wpUpdate, wpDelete, wpComment, wpActivities, wpAllowed)
}

var wpColumns = []Column{
	{Header: "ID", Key: "id"},
	{Header: "TYPE", Key: "type"},
	{Header: "STATUS", Key: "status"},
	{Header: "SUBJECT", Key: "subject"},
	{Header: "ASSIGNEE", Key: "assignee"},
	{Header: "PRIORITY", Key: "priority"},
	{Header: "PROJECT", Key: "project"},
	{Header: "DUE", Key: "dueDate"},
}

var wpList = &Op{
	Tool:    "op_list_work_packages",
	Cmd:     []string{"wp", "list"},
	Aliases: []string{"ls", "search"},
	Short:   "List and search work packages with filters",
	Long: `List work packages. Filters combine with AND. By default only open work
packages are shown, in the configured default project (if any), most
recently updated first.

--status accepts: open (default), closed, all, or a comma-separated list of
status names/ids ("New,In progress"). --assignee/--author/--responsible accept
me, none, a login, email, name or id. --search does full-text search over
subject, description and comments.

--filter takes raw API filters (JSON) for anything not covered by flags, e.g.
  [{"dueDate":{"operator":"<t+","values":["7"]}}]
See 'opcli help filters' for operators. --query-id runs a saved query instead.`,
	Example: `  opcli wp list                                  # open WPs, default project
  opcli wp list --assignee me --project all      # my open work, everywhere
  opcli wp list -p demo-project --type Bug --status all
  opcli wp list --search "login timeout" --max 10
  opcli wp list --status "In progress,On hold" --sort dueDate:asc
  opcli wp list --filter '[{"dueDate":{"operator":"<t+","values":["7"]}}]'
  opcli wp list --query-id 12                    # run a saved query
  opcli wp list --assignee me -o json            # machine-readable`,
	Params: []Param{
		pProject("Project id, identifier or name; 'all' for every project (default: configured project)"),
		{Name: "status", Kind: String, Short: "s", Default: "open", Desc: "open, closed, all, or comma-separated status names/ids"},
		{Name: "type", Kind: String, Short: "t", Desc: "Comma-separated type names/ids (Task, Bug, Feature...)"},
		{Name: "assignee", Kind: String, Short: "a", Desc: "me, none, or login/email/name/id"},
		{Name: "author", Kind: String, Desc: "me, or login/email/name/id"},
		{Name: "responsible", Kind: String, Desc: "Accountable user: me, none, or login/email/name/id"},
		{Name: "priority", Kind: String, Desc: "Comma-separated priority names/ids"},
		{Name: "version", Kind: String, Desc: "Target version name or id (name needs a project)"},
		{Name: "parent", Kind: String, Desc: "Only direct children of this work package id"},
		{Name: "search", Kind: String, Short: "q", Desc: "Full-text search in subject, description and comments"},
		{Name: "filter", Kind: String, Desc: "Extra raw API filters as a JSON array (see 'opcli help filters')"},
		{Name: "sort", Kind: String, Default: "updatedAt:desc", Desc: "Sort spec field:asc|desc, comma-separated (id, subject, updatedAt, dueDate, priority, status...)"},
		{Name: "query_id", Kind: Int, Desc: "Run a saved query by id (other filters are ignored)"},
		pMax(50), pPage,
	},
	Columns: wpColumns,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if qid := a.Int("query_id"); qid > 0 {
			return e.runSavedQuery(ctx, qid, a)
		}
		f := NewFilters()
		project := a.String("project")
		projectID := 0
		if !strings.EqualFold(project, "all") && (project != "" || e.DefaultProject != "") {
			var err error
			if projectID, err = e.ProjectID(ctx, project); err != nil {
				return nil, err
			}
			f = f.Add("project", "=", projectID)
		}

		switch st := strings.ToLower(a.String("status")); st {
		case "", "open":
			f = f.Add("status", "o")
		case "closed":
			f = f.Add("status", "c")
		case "all", "*":
		default:
			ids, err := e.resolveMany(ctx, a.Strings("status"), e.StatusID)
			if err != nil {
				return nil, err
			}
			f = f.Add("status", "=", ids...)
		}
		if a.Has("type") {
			ids, err := e.resolveMany(ctx, a.Strings("type"), e.TypeID)
			if err != nil {
				return nil, err
			}
			f = f.Add("type", "=", ids...)
		}
		if a.Has("priority") {
			ids, err := e.resolveMany(ctx, a.Strings("priority"), e.PriorityID)
			if err != nil {
				return nil, err
			}
			f = f.Add("priority", "=", ids...)
		}
		for _, field := range []string{"assignee", "author", "responsible"} {
			if !a.Has(field) {
				continue
			}
			var err error
			if f, err = e.userFilter(ctx, f, field, a.String(field)); err != nil {
				return nil, err
			}
		}
		if a.Has("version") {
			vid, err := e.VersionID(ctx, projectID, a.String("version"))
			if err != nil {
				return nil, err
			}
			f = f.Add("targetVersion", "=", vid)
		}
		if a.Has("parent") {
			pid, err := WorkPackageID(a.String("parent"))
			if err != nil {
				return nil, err
			}
			f = f.Add("parent", "=", pid)
		}
		if s := a.String("search"); s != "" {
			f = f.Add("search", "**", s)
		}
		f, err := f.Merge(a.String("filter"))
		if err != nil {
			return nil, err
		}
		q := url.Values{"filters": {f.JSON()}}
		if s := a.String("sort"); s != "" {
			sb, err := sortBy(s)
			if err != nil {
				return nil, err
			}
			q.Set("sortBy", sb)
		}
		l, err := e.list(ctx, "/work_packages", q, a)
		if err != nil {
			return nil, err
		}
		e.decorateWPs(l.Items)
		return l, nil
	},
}

func (e *Env) runSavedQuery(ctx context.Context, id int, a Args) (*List, error) {
	q := url.Values{}
	max := a.Int("max")
	if max <= 0 {
		max = 1000
	}
	q.Set("pageSize", strconv.Itoa(max))
	q.Set("offset", strconv.Itoa(max1(a.Int("page"))))
	res, err := e.C.Get(ctx, fmt.Sprintf("/queries/%d", id), q)
	if err != nil {
		return nil, err
	}
	emb, _ := res["_embedded"].(map[string]any)
	results, _ := emb["results"].(map[string]any)
	l := NewList(hal.FlattenAll(client.Elements(results)), intOf(results["total"]))
	e.decorateWPs(l.Items)
	return l, nil
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func (e *Env) userFilter(ctx context.Context, f Filters, field, ref string) (Filters, error) {
	switch strings.ToLower(ref) {
	case "none":
		return f.Add(field, "!*"), nil
	case "me":
		return f.Add(field, "=", "me"), nil
	}
	id, err := e.UserID(ctx, ref)
	if err != nil {
		return nil, err
	}
	return f.Add(field, "=", id), nil
}

func (e *Env) resolveMany(ctx context.Context, refs []string, fn func(context.Context, string) (int, error)) ([]any, error) {
	out := make([]any, 0, len(refs))
	for _, r := range refs {
		id, err := fn(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// Fields dropped from work packages in listings to keep them lean (the
// description and the rest are available through op_get_work_package).
var wpListNoise = map[string]bool{
	"description": true, "laborCosts": true, "position": true, "hasProjectAttributes": true,
	"ignoreNonWorkingDays": true, "scheduleManually": true, "displayId": true, "lockVersion": true,
	"derivedStartDate": true, "derivedDueDate": true, "derivedEstimatedTime": true,
	"derivedRemainingTime": true, "derivedPercentageDone": true, "backlogBucket": true,
}

// decorateWPs adds the browser URL and drops empty and noisy fields.
func (e *Env) decorateWPs(items []map[string]any) {
	for _, it := range items {
		e.decorateWP(it)
		for k, v := range it {
			if v == nil || v == "" || wpListNoise[k] {
				delete(it, k)
			}
		}
	}
}

func (e *Env) decorateWP(it map[string]any) {
	if id := intOf(it["id"]); id > 0 {
		it["url"] = e.C.WebURL(fmt.Sprintf("/work_packages/%d", id))
	}
	delete(it, "_type")
}

var wpGet = &Op{
	Tool:    "op_get_work_package",
	Cmd:     []string{"wp", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a work package with description, relations, children and attachments",
	Long: `Show one work package: all its fields, the Markdown description, relations,
children, attachments and (with --activities) the comment/change history.`,
	Example: `  opcli wp get 42
  opcli wp get 42 --activities
  opcli wp get 42 -o json`,
	Params: []Param{
		pID("Work package"),
		{Name: "activities", Kind: Bool, Desc: "Also include comments and change history"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
		if err != nil {
			return nil, err
		}
		wp := hal.Flatten(res, "relations", "attachments")
		e.decorateWP(wp)
		if links, ok := res["_links"].(map[string]any); ok {
			if ch, ok := links["children"].([]any); ok && len(ch) > 0 {
				var children []map[string]any
				for _, c := range ch {
					m, _ := c.(map[string]any)
					children = append(children, map[string]any{"id": hal.IDFromHref(fmt.Sprint(m["href"])), "subject": m["title"]})
				}
				wp["children"] = children
			}
		}
		if rels, ok := wp["relations"].([]map[string]any); ok {
			wp["relations"] = compactRelations(rels, id)
		}
		if atts, ok := wp["attachments"].([]map[string]any); ok {
			wp["attachments"] = compactAttachments(atts)
		}
		if a.Bool("activities") {
			acts, err := e.activities(ctx, id)
			if err != nil {
				return nil, err
			}
			wp["activities"] = acts
		}
		return wp, nil
	},
}

func compactRelations(rels []map[string]any, self int) []map[string]any {
	out := make([]map[string]any, 0, len(rels))
	for _, r := range rels {
		typ, other, otherID := r["type"], r["to"], r["toId"]
		if intOf(r["toId"]) == self {
			typ, other, otherID = r["reverseType"], r["from"], r["fromId"]
		}
		out = append(out, map[string]any{"id": r["id"], "type": typ, "workPackageId": otherID, "subject": other})
	}
	return out
}

func compactAttachments(atts []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(atts))
	for _, at := range atts {
		out = append(out, map[string]any{"id": at["id"], "fileName": at["fileName"], "fileSize": at["fileSize"], "contentType": at["contentType"], "author": at["author"], "createdAt": at["createdAt"]})
	}
	return out
}

// wpFieldParams are shared by create and update.
var wpFieldParams = []Param{
	{Name: "type", Kind: String, Short: "t", Desc: "Type name or id (Task, Bug, Feature, Epic, Milestone...)"},
	{Name: "description", Kind: String, Short: "d", File: true, Desc: "Description in Markdown"},
	{Name: "status", Kind: String, Short: "s", Desc: "Status name or id (see 'opcli wp allowed <id>')"},
	{Name: "priority", Kind: String, Desc: "Priority name or id"},
	{Name: "assignee", Kind: String, Short: "a", Desc: "me, none, or login/email/name/id"},
	{Name: "responsible", Kind: String, Desc: "Accountable user: me, none, or login/email/name/id"},
	{Name: "parent", Kind: String, Desc: "Parent work package id ('none' to detach)"},
	{Name: "version", Kind: String, Desc: "Target version name or id ('none' to clear)"},
	{Name: "category", Kind: String, Desc: "Category name or id ('none' to clear)"},
	{Name: "start_date", Kind: String, Desc: "Start date: YYYY-MM-DD, today, tomorrow, +3d... ('none' to clear)"},
	{Name: "due_date", Kind: String, Desc: "Finish date: YYYY-MM-DD, today, +7d... ('none' to clear)"},
	{Name: "estimate", Kind: String, Desc: "Estimated work: 4, 1.5, 90m, 2h30m or PT2H ('none' to clear)"},
	{Name: "remaining", Kind: String, Desc: "Remaining work, same formats as --estimate"},
	{Name: "percent_done", Kind: Int, Desc: "% complete (0-100), when progress is work-based"},
	{Name: "fields", Kind: String, Desc: `Raw JSON object merged into the request, for custom fields or anything else, e.g. {"customField3":"x"}`},
	{Name: "notify", Kind: Bool, Default: true, Desc: "Send email notifications for this change"},
}

var wpCreate = &Op{
	Tool:    "op_create_work_package",
	Cmd:     []string{"wp", "create"},
	Aliases: []string{"new", "add"},
	Short:   "Create a work package",
	Long: `Create a work package. Only --subject is required: the project falls back to
the configured default and the type to Task. Long descriptions are easier
to pass with --description-file (use - for stdin).`,
	Example: `  opcli wp create --subject "Fix login timeout" --type Bug --priority High
  opcli wp create -p demo-project --subject "Write docs" --assignee me --due-date +7d
  opcli wp create --subject "Sub-task" --parent 42 --estimate 3h
  cat spec.md | opcli wp create --subject "Epic: SSO" --type Epic --description-file -`,
	Params: append([]Param{
		pProject(""),
		{Name: "subject", Kind: String, Required: true, Desc: "Title of the work package"},
	}, wpFieldParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		if !a.Has("type") {
			a["type"] = "Task"
		}
		body, err := e.wpPayload(ctx, a, pid)
		if err != nil {
			return nil, err
		}
		body["subject"] = a.String("subject")
		body["_links"].(map[string]any)["project"] = hal.Ref(href("projects", pid))
		q := url.Values{}
		if !a.Bool("notify") {
			q.Set("notify", "false")
		}
		res, err := e.C.JSON(ctx, "POST", "/work_packages", q, body)
		if err != nil {
			return nil, err
		}
		wp := hal.Flatten(res)
		e.decorateWP(wp)
		return wp, nil
	},
}

var wpUpdate = &Op{
	Tool:    "op_update_work_package",
	Cmd:     []string{"wp", "update"},
	Aliases: []string{"edit", "set"},
	Short:   "Update fields of a work package (status, assignee, dates, ...)",
	Long: `Update only the fields you pass. The lock version is handled for you (a
concurrent edit triggers one automatic retry). Pass 'none' to clear user,
parent, version, category, dates or estimates. --comment adds a comment in
the same call. --project moves the work package to another project.`,
	Example: `  opcli wp update 42 --status "In progress" --assignee me
  opcli wp update 42 --status Closed --comment "Fixed in v1.2"
  opcli wp update 42 --due-date 2026-10-15 --estimate 6h
  opcli wp update 42 --assignee none --version none
  opcli wp update 42 --fields '{"customField3":"ACME"}'`,
	Params: append([]Param{
		pID("Work package"),
		{Name: "subject", Kind: String, Desc: "New title"},
		{Name: "project", Kind: String, Desc: "Move to this project (id, identifier or name)"},
		{Name: "comment", Kind: String, File: true, Desc: "Also add this comment (Markdown)"},
	}, wpFieldParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		var res map[string]any
		for attempt := 0; attempt < 2; attempt++ {
			cur, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
			if err != nil {
				return nil, err
			}
			pid := intOf(hal.IDFromHref(hal.Link(cur, "project")))
			if a.Has("project") {
				if pid, err = e.ProjectID(ctx, a.String("project")); err != nil {
					return nil, err
				}
			}
			body, err := e.wpPayload(ctx, a, pid)
			if err != nil {
				return nil, err
			}
			if a.Has("subject") {
				body["subject"] = a.String("subject")
			}
			if a.Has("project") {
				body["_links"].(map[string]any)["project"] = hal.Ref(href("projects", pid))
			}
			body["lockVersion"] = cur["lockVersion"]
			if len(body) == 2 && len(body["_links"].(map[string]any)) == 0 {
				res = cur // only a comment requested
				break
			}
			q := url.Values{}
			if !a.Bool("notify") {
				q.Set("notify", "false")
			}
			res, err = e.C.JSON(ctx, "PATCH", fmt.Sprintf("/work_packages/%d", id), q, body)
			if client.IsConflict(err) && attempt == 0 {
				continue
			}
			if err != nil {
				return nil, err
			}
			break
		}
		if c := a.String("comment"); c != "" {
			if _, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/activities", id), map[string]any{"comment": hal.Text(c)}); err != nil {
				return nil, fmt.Errorf("fields updated but adding the comment failed: %w", err)
			}
		}
		wp := hal.Flatten(res)
		e.decorateWP(wp)
		return wp, nil
	},
}

// wpPayload builds the create/update body from the shared field params.
func (e *Env) wpPayload(ctx context.Context, a Args, projectID int) (map[string]any, error) {
	body := map[string]any{}
	links := map[string]any{}
	body["_links"] = links

	if a.Has("description") {
		body["description"] = hal.Text(a.String("description"))
	}
	type resolver func(context.Context, string) (int, error)
	for _, r := range []struct {
		param, link, kind string
		fn                resolver
	}{
		{"type", "type", "types", e.TypeID},
		{"status", "status", "statuses", e.StatusID},
		{"priority", "priority", "priorities", e.PriorityID},
	} {
		if !a.Has(r.param) {
			continue
		}
		id, err := r.fn(ctx, a.String(r.param))
		if err != nil {
			return nil, err
		}
		links[r.link] = hal.Ref(href(r.kind, id))
	}
	for _, u := range []string{"assignee", "responsible"} {
		if !a.Has(u) {
			continue
		}
		h, err := e.UserHref(ctx, a.String(u))
		if err != nil {
			return nil, err
		}
		links[u] = hal.Ref(h)
	}
	if a.Has("parent") {
		p := a.String("parent")
		if p == "" || strings.EqualFold(p, "none") {
			links["parent"] = hal.Ref("")
		} else {
			pid, err := WorkPackageID(p)
			if err != nil {
				return nil, err
			}
			links["parent"] = hal.Ref(href("work_packages", pid))
		}
	}
	if a.Has("version") {
		v := a.String("version")
		if v == "" || strings.EqualFold(v, "none") {
			links["targetVersions"] = []any{}
		} else {
			vid, err := e.VersionID(ctx, projectID, v)
			if err != nil {
				return nil, err
			}
			links["targetVersions"] = []any{hal.Ref(href("versions", vid))}
		}
	}
	if a.Has("category") {
		c := a.String("category")
		if c == "" || strings.EqualFold(c, "none") {
			links["category"] = hal.Ref("")
		} else {
			cid, err := e.CategoryID(ctx, projectID, c)
			if err != nil {
				return nil, err
			}
			links["category"] = hal.Ref(href("categories", cid))
		}
	}
	for param, field := range map[string]string{"start_date": "startDate", "due_date": "dueDate"} {
		if !a.Has(param) {
			continue
		}
		d, err := dateOrClear(a, param)
		if err != nil {
			return nil, err
		}
		body[field] = d
	}
	for param, field := range map[string]string{"estimate": "estimatedTime", "remaining": "remainingTime"} {
		if !a.Has(param) {
			continue
		}
		v := a.String(param)
		if v == "" || strings.EqualFold(v, "none") {
			body[field] = nil
			continue
		}
		h, err := hal.ParseHours(v)
		if err != nil {
			return nil, err
		}
		body[field] = hal.ISOHours(h)
	}
	if a.Has("percent_done") {
		body["percentageDone"] = a.Int("percent_done")
	}
	if raw := a.String("fields"); raw != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return nil, fmt.Errorf("invalid --fields JSON object: %w", err)
		}
		for k, v := range extra {
			if k == "_links" {
				if m, ok := v.(map[string]any); ok {
					for lk, lv := range m {
						links[lk] = lv
					}
				}
				continue
			}
			body[k] = v
		}
	}
	return body, nil
}

var wpDelete = &Op{
	Tool:    "op_delete_work_package",
	Cmd:     []string{"wp", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete a work package (irreversible, children included)",
	Example: `  opcli wp delete 42`,
	Params:  []Param{pID("Work package")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		if err := e.C.Delete(ctx, fmt.Sprintf("/work_packages/%d", id)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("Deleted work package #%d", id)}, nil
	},
}

var wpComment = &Op{
	Tool:  "op_comment_work_package",
	Cmd:   []string{"wp", "comment"},
	Short: "Add a comment to a work package",
	Long:  `Add a Markdown comment. Mention users with the OpenProject syntax if needed.`,
	Example: `  opcli wp comment 42 --body "Deployed to staging"
  git log -1 --format=%B | opcli wp comment 42 --body-file -`,
	Params: []Param{
		pID("Work package"),
		{Name: "body", Kind: String, Required: true, File: true, Short: "b", Desc: "Comment text (Markdown)"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/activities", id), map[string]any{"comment": hal.Text(a.String("body"))})
		if err != nil {
			return nil, err
		}
		act := flattenActivity(res)
		e.nameUser(ctx, act, map[int]string{})
		return act, nil
	},
}

var wpActivities = &Op{
	Tool:    "op_list_work_package_activities",
	Cmd:     []string{"wp", "activities"},
	Aliases: []string{"history", "comments"},
	Short:   "Show comments and change history of a work package",
	Example: `  opcli wp activities 42
  opcli wp activities 42 --comments-only`,
	Params: []Param{
		pID("Work package"),
		{Name: "comments_only", Kind: Bool, Desc: "Only entries with a comment"},
	},
	Columns: []Column{
		{Header: "ID", Key: "id"}, {Header: "DATE", Key: "createdAt"}, {Header: "USER", Key: "user"},
		{Header: "COMMENT", Key: "comment"}, {Header: "CHANGES", Key: "details"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		acts, err := e.activities(ctx, id)
		if err != nil {
			return nil, err
		}
		if a.Bool("comments_only") {
			var only []map[string]any
			for _, it := range acts {
				if s, _ := it["comment"].(string); s != "" {
					only = append(only, it)
				}
			}
			acts = only
		}
		return NewList(acts, len(acts)), nil
	},
}

func (e *Env) activities(ctx context.Context, wpID int) ([]map[string]any, error) {
	items, _, err := e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/activities", wpID), nil, 0)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	names := map[int]string{}
	for _, it := range items {
		act := flattenActivity(it)
		e.nameUser(ctx, act, names)
		out = append(out, act)
	}
	return out, nil
}

// nameUser fills "user" from "userId": activity links carry no title.
func (e *Env) nameUser(ctx context.Context, act map[string]any, names map[int]string) {
	uid := intOf(act["userId"])
	if uid == 0 || act["user"] != nil {
		return
	}
	if _, ok := names[uid]; !ok {
		names[uid] = fmt.Sprint(uid)
		if u, err := e.C.Get(ctx, fmt.Sprintf("/users/%d", uid), nil); err == nil {
			names[uid] = fmt.Sprint(u["name"])
		}
	}
	act["user"] = names[uid]
}

func flattenActivity(res map[string]any) map[string]any {
	a := hal.Flatten(res)
	if raw, ok := res["details"].([]any); ok {
		var details []any
		for _, d := range raw {
			if m, ok := d.(map[string]any); ok {
				details = append(details, m["raw"])
			}
		}
		a["details"] = details
	}
	delete(a, "_type")
	delete(a, "emojiReactions")
	return a
}

var wpAllowed = &Op{
	Tool:  "op_list_work_package_allowed_values",
	Cmd:   []string{"wp", "allowed"},
	Short: "Show values a work package accepts: statuses, types, priorities, versions...",
	Long: `Ask OpenProject which values each field of a work package accepts right now.
Statuses reflect the workflow (valid transitions for this type and your role).
Without --field all fields with a finite list of values are shown. Pass
--field assignee to list the users that can be assigned.`,
	Example: `  opcli wp allowed 42 --field status
  opcli wp allowed 42 --field assignee
  opcli wp allowed 42 -o json`,
	Params: []Param{
		pID("Work package"),
		{Name: "field", Kind: String, Short: "f", Desc: "Only this field (status, type, priority, category, version, assignee, responsible, project, projectPhase)"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		cur, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
		if err != nil {
			return nil, err
		}
		form, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/form", id), map[string]any{"lockVersion": cur["lockVersion"]})
		if err != nil {
			return nil, err
		}
		emb, _ := form["_embedded"].(map[string]any)
		schema, _ := emb["schema"].(map[string]any)
		want := a.String("field")
		if strings.EqualFold(want, "version") {
			want = "targetVersions"
		}
		out := map[string]any{}
		for key, raw := range schema {
			def, ok := raw.(map[string]any)
			if !ok || def["writable"] != true || (want != "" && !strings.EqualFold(key, want)) {
				continue
			}
			var values []map[string]any
			if e2, ok := def["_embedded"].(map[string]any); ok {
				if av, ok := e2["allowedValues"].([]any); ok {
					for _, v := range av {
						if m, ok := v.(map[string]any); ok {
							values = append(values, map[string]any{"id": m["id"], "name": firstNonNil(m["name"], m["subject"], m["title"])})
						}
					}
				}
			} else if want != "" {
				links, _ := def["_links"].(map[string]any)
				avl, _ := links["allowedValues"].(map[string]any)
				if h, _ := avl["href"].(string); h != "" {
					items, _, err := e.C.Collect(ctx, h, nil, 200)
					if err != nil {
						return nil, err
					}
					for _, m := range items {
						values = append(values, map[string]any{"id": m["id"], "name": firstNonNil(m["name"], m["subject"], m["title"])})
					}
				}
			}
			if values != nil || want != "" {
				if values == nil {
					values = []map[string]any{}
				}
				out[key] = values
			}
		}
		if want != "" && len(out) == 0 {
			return nil, fmt.Errorf("field %q is not writable or unknown for work package #%d", a.String("field"), id)
		}
		return out, nil
	},
	Text: renderAllowed,
}

func firstNonNil(vs ...any) any {
	for _, v := range vs {
		if v != nil {
			return v
		}
	}
	return nil
}
