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

	register(wpList, wpGet, wpCreate, wpUpdate, wpDelete, wpComment, wpCommentEdit, wpActivities, wpAllowed, wpFieldsOp)
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

--field filters by any field by its name, custom fields included
("Customer=ACME"; text fields match by "contains", lists, users and versions
by name, "NAME=" finds empty ones). It needs a project. Custom fields come
back by name under "customFields".

--filter takes raw API filters (JSON) for anything not covered by flags, e.g.
  [{"dueDate":{"operator":"<t+","values":["7"]}}]
See 'opcli help filters' for operators. --query-id runs a saved query instead.`,
	Example: `  opcli wp list                                  # open WPs, default project
  opcli wp list --assignee me --project all      # my open work, everywhere
  opcli wp list -p demo-project --type Bug --status all
  opcli wp list --search "login timeout" --max 10
  opcli wp list --status "In progress,On hold" --sort dueDate:asc
  opcli wp list -p demo-project --field "Customer=ACME" --field "Billable=yes"
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
		{Name: "field", Kind: Strings, Desc: `Filter by any field by name, NAME=VALUE (repeatable): "Customer=ACME", "Area=Backend,Ops"; "NAME=" means empty; needs a project`},
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
		f, err := e.fieldFilters(ctx, f, rawStrings(a["field"]), projectID)
		if err != nil {
			return nil, err
		}
		if f, err = f.Merge(a.String("filter")); err != nil {
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
			if strings.Contains(err.Error(), "filter does not exist") && a.Has("field") {
				return nil, fmt.Errorf("%w: OpenProject cannot filter by this field (user and version custom fields are not filterable); filter by another field and check the value with jq", err)
			}
			return nil, err
		}
		e.decorateWPs(ctx, l.Items)
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
	e.decorateWPs(ctx, l.Items)
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
// description and the rest are available through wp get).
var wpListNoise = map[string]bool{
	"description": true, "laborCosts": true, "position": true, "hasProjectAttributes": true,
	"ignoreNonWorkingDays": true, "scheduleManually": true, "displayId": true, "lockVersion": true,
	"derivedStartDate": true, "derivedDueDate": true, "derivedEstimatedTime": true,
	"derivedRemainingTime": true, "derivedPercentageDone": true, "backlogBucket": true,
}

// decorateWPs adds the browser URL, names custom fields and drops empty and
// noisy fields.
func (e *Env) decorateWPs(ctx context.Context, items []map[string]any) {
	for _, it := range items {
		e.decorateWP(ctx, it)
		compactWP(it)
	}
}

func (e *Env) decorateWP(ctx context.Context, it map[string]any) {
	if id := intOf(it["id"]); id > 0 {
		it["url"] = e.C.WebURL(fmt.Sprintf("/work_packages/%d", id))
	}
	delete(it, "_type")
	e.customFieldLabels(ctx, []map[string]any{it})
}

var wpGet = &Op{
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
		e.decorateWP(ctx, wp)
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
	{Name: "type", Kind: String, Short: "t", Desc: "Type name or id (Task, Bug, Feature, Epic, Milestone...); default: config default_type, else the project's default"},
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
	pField,
	{Name: "fields", Kind: String, Desc: `Raw JSON object merged into the request, e.g. {"customField3":"x"}; prefer --field`},
	{Name: "notify", Kind: Bool, Default: true, Desc: "Send email notifications for this change"},
}

var wpCreate = &Op{
	Cmd:     []string{"wp", "create"},
	Aliases: []string{"new", "add"},
	Short:   "Create a work package",
	Long: `Create a work package. Only --subject is required: the project falls back to
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
request. @login in the description becomes a mention that notifies the user.`,
	Example: `  opcli wp create --subject "Fix login timeout" --type Bug --priority High
  opcli wp create -p demo-project --subject "Write docs" --assignee me --due-date +7d
  opcli wp create --subject "Sub-task" --parent 42 --estimate 3h
  opcli wp create --subject "Onboarding" --field "Customer=ACME" --field "Area=Backend"
  opcli wp create --subject "Crash on save" --type Bug --description-file bug.md --attach crash.log
  cat spec.md | opcli wp create --subject "Epic: SSO" --type Epic --description-file -`,
	Params: append([]Param{
		pProject(""),
		{Name: "subject", Kind: String, Required: true, Desc: "Title of the work package"},
		pAttach,
	}, wpFieldParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		men, err := e.mentionArgs(ctx, a, "description")
		if err != nil {
			return nil, err
		}
		files, err := collectFiles(a, "description")
		if err != nil {
			return nil, err
		}
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		if !a.Has("type") {
			tid, err := e.defaultTypeID(ctx, pid)
			if err != nil {
				return nil, err
			}
			a["type"] = strconv.Itoa(tid)
		}
		body, err := e.wpPayload(ctx, a, pid)
		if err != nil {
			return nil, err
		}
		body["subject"] = a.String("subject")
		links := body["_links"].(map[string]any)
		links["project"] = hal.Ref(href("projects", pid))
		formPath := fmt.Sprintf("/projects/%d/work_packages/form", pid)
		if err := e.wpFields(ctx, formPath, body, a, pid); err != nil {
			return nil, err
		}
		if err := e.validate(ctx, formPath, body); err != nil {
			return nil, err
		}
		// Files go up without a container and are claimed by the create,
		// so a failed upload leaves no half-made work package behind.
		t := e.newAttacher(0)
		if !files.empty() {
			texts, err := files.apply(ctx, t, a)
			if err != nil {
				return nil, err
			}
			if d, ok := texts["description"]; ok {
				body["description"] = hal.Text(d)
			}
			links["attachments"] = t.claims()
		}
		q := url.Values{}
		if !a.Bool("notify") {
			q.Set("notify", "false")
		}
		res, err := e.C.JSON(ctx, "POST", "/work_packages", q, body)
		if err != nil {
			t.discard(ctx)
			return nil, err
		}
		wp := hal.Flatten(res)
		e.decorateWP(ctx, wp)
		if len(t.Uploaded) > 0 {
			wp["uploaded"] = t.Uploaded
		}
		men.report(wp)
		return wp, nil
	},
}

// wpFields applies --field, resolving names against the form at formPath.
func (e *Env) wpFields(ctx context.Context, formPath string, body map[string]any, a Args, projectID int) error {
	specs := rawStrings(a["field"])
	if len(specs) == 0 {
		return nil
	}
	defs, _, err := e.form(ctx, formPath, body)
	if err != nil {
		return err
	}
	return e.applyFields(ctx, body, defs, specs, projectID)
}

var wpUpdate = &Op{
	Cmd:     []string{"wp", "update"},
	Aliases: []string{"edit", "set"},
	Short:   "Update fields of a work package (status, assignee, dates, ...)",
	Long: `Update only the fields you pass. The lock version is handled for you (a
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
ones that were updated. Run it with --dry-run first.`,
	Example: `  opcli wp update 42 --status "In progress" --assignee me
  opcli wp update 42 43 44 --version "Sprint 7" --dry-run
  opcli wp update 42 --status Closed --comment "Fixed in v1.2"
  opcli wp update 42 --comment-file review.md --attach report.pdf
  opcli wp update 42 --due-date 2026-10-15 --estimate 6h
  opcli wp update 42 --assignee none --version none
  opcli wp update 42 --field "Customer=ACME" --field "Tags=urgent,customer"
  opcli wp update 42 --field "Customer="          # clear a custom field`,
	Params: append([]Param{
		{Name: "id", Kind: String, Required: true, Positional: true, Rest: true, Desc: "Work package id, or several (42 43 44) to apply the same change to each"},
		{Name: "subject", Kind: String, Desc: "New title"},
		{Name: "project", Kind: String, Desc: "Move to this project (id, identifier or name)"},
		{Name: "comment", Kind: String, File: true, Desc: "Also add this comment (Markdown)"},
		pInternal,
		pAttach,
	}, wpFieldParams...),
	Safety:  Write,
	Columns: wpColumns,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		ids, err := workPackageIDs(a.String("id"))
		if err != nil {
			return nil, err
		}
		if len(ids) == 1 {
			return e.updateWP(ctx, a, ids[0])
		}
		var done []map[string]any
		var failed []string
		for _, id := range ids {
			wa := make(Args, len(a))
			for k, v := range a {
				wa[k] = v
			}
			wp, err := e.updateWP(ctx, wa, id)
			if err != nil {
				failed = append(failed, fmt.Sprintf("#%d: %v", id, err))
				continue
			}
			done = append(done, compactWP(wp))
		}
		if len(failed) > 0 {
			ok := make([]string, len(done))
			for i, wp := range done {
				ok[i] = fmt.Sprintf("#%v", wp["id"])
			}
			return nil, fmt.Errorf("%d of %d work packages not updated: %s (updated: %s)",
				len(failed), len(ids), strings.Join(failed, "; "), strings.Join(ok, ", "))
		}
		return NewList(done, len(done)), nil
	},
}

// workPackageIDs parses "42", "42 43", "42,#43".
func workPackageIDs(ref string) ([]int, error) {
	var ids []int
	for _, f := range strings.FieldsFunc(ref, func(r rune) bool { return r == ',' || r == ' ' }) {
		id, err := WorkPackageID(f)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("missing work package id")
	}
	return ids, nil
}

// compactWP drops what listings drop (see wpListNoise).
func compactWP(wp map[string]any) map[string]any {
	for k, v := range wp {
		if v == nil || v == "" || wpListNoise[k] {
			delete(wp, k)
		}
	}
	return wp
}

// updateWP applies the wp update params to one work package.
func (e *Env) updateWP(ctx context.Context, a Args, id int) (map[string]any, error) {
	men, err := e.mentionArgs(ctx, a, "description", "comment")
	if err != nil {
		return nil, err
	}
	files, err := collectFiles(a, "description", "comment")
	if err != nil {
		return nil, err
	}
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
	formPath := fmt.Sprintf("/work_packages/%d/form", id)
	if err := e.wpFields(ctx, formPath, body, a, pid); err != nil {
		return nil, err
	}
	onlyComment := len(body) == 2 && len(body["_links"].(map[string]any)) == 0
	if !onlyComment {
		if err := e.validate(ctx, formPath, body); err != nil {
			return nil, err
		}
	}

	t := e.newAttacher(id)
	if !files.empty() {
		texts, err := files.apply(ctx, t, a)
		if err != nil {
			return nil, err
		}
		for p, v := range texts {
			a[p] = v
		}
		if d, ok := texts["description"]; ok {
			body["description"] = hal.Text(d)
		}
	}
	res := cur
	if !onlyComment {
		q := url.Values{}
		if !a.Bool("notify") {
			q.Set("notify", "false")
		}
		for attempt := 0; ; attempt++ {
			res, err = e.C.JSON(ctx, "PATCH", fmt.Sprintf("/work_packages/%d", id), q, body)
			if !client.IsConflict(err) || attempt > 0 {
				break
			}
			fresh, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
			if err != nil {
				return nil, err
			}
			body["lockVersion"] = fresh["lockVersion"]
		}
		if err != nil {
			return nil, err
		}
	}
	if c := a.String("comment"); c != "" {
		if _, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/activities", id), commentBody(c, a)); err != nil {
			return nil, fmt.Errorf("fields updated but adding the comment failed: %w", err)
		}
	}
	wp := hal.Flatten(res)
	e.decorateWP(ctx, wp)
	if len(t.Uploaded) > 0 {
		wp["uploaded"] = t.Uploaded
	}
	men.report(wp)
	return wp, nil
}

var pInternal = Param{Name: "internal", Kind: Bool, Desc: "Internal comment, visible only to roles allowed to see internal comments (Enterprise; enabled per project)"}

func commentBody(text string, a Args) map[string]any {
	b := map[string]any{"comment": hal.Text(text)}
	if a.Bool("internal") {
		b["internal"] = true
	}
	return b
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
	Cmd:   []string{"wp", "comment"},
	Short: "Add a comment to a work package",
	Long: `Add a Markdown comment. Local images (![alt](shot.png)) are uploaded as
attachments of the work package and rendered inline; relative paths resolve
against --body-file (or the working directory). @login becomes a mention
that notifies the user (the result lists "mentioned", and
"unresolvedMentions" for what matched nobody). Fix a comment later with
'wp comment-edit'.`,
	Example: `  opcli wp comment 42 --body "Deployed to staging"
  git log -1 --format=%B | opcli wp comment 42 --body-file -
  opcli wp comment 42 --body-file notes/review.md   # with ![](shot.png) inside
  opcli wp comment 42 --body "@jane can you review?"
  opcli wp comment 42 --body "Customer budget is tight" --internal`,
	Params: []Param{
		pID("Work package"),
		{Name: "body", Kind: String, Required: true, File: true, Short: "b", Desc: "Comment text (Markdown)"},
		pInternal,
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		return e.writeComment(ctx, id, a, func(body string) (map[string]any, error) {
			return e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/activities", id), commentBody(body, a))
		})
	},
}

var wpCommentEdit = &Op{
	Cmd:   []string{"wp", "comment-edit"},
	Short: "Replace the text of a comment",
	Long: `Replace the text of an existing comment. The id is the activity id shown
by 'wp activities' (or 'wp get --activities'). Only your own comments can be
edited unless your role allows editing others'. Comments cannot be deleted
through the API. Local images are handled as in 'wp comment'.`,
	Example: `  opcli wp comment-edit 1234 --body "Deployed to production"
  opcli wp comment-edit 1234 --body-file fixed.md`,
	Params: []Param{
		pID("Activity (comment)"),
		{Name: "body", Kind: String, Required: true, File: true, Short: "b", Desc: "New comment text (Markdown)"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		aid := a.String("id")
		cur, err := e.C.Get(ctx, "/activities/"+url.PathEscape(aid), nil)
		if err != nil {
			return nil, err
		}
		wpID := intOf(hal.IDFromHref(hal.Link(cur, "workPackage")))
		return e.writeComment(ctx, wpID, a, func(body string) (map[string]any, error) {
			// The spec documents an object, but the server only accepts a
			// plain string here ({"raw": …} is "comment is invalid").
			return e.C.Patch(ctx, "/activities/"+url.PathEscape(aid), map[string]any{"comment": body})
		})
	},
}

// writeComment uploads the local images of --body to the work package and
// sends the rewritten text with send.
func (e *Env) writeComment(ctx context.Context, wpID int, a Args, send func(string) (map[string]any, error)) (any, error) {
	men, err := e.mentionArgs(ctx, a, "body")
	if err != nil {
		return nil, err
	}
	files, err := collectFiles(a, "body")
	if err != nil {
		return nil, err
	}
	body := a.String("body")
	t := e.newAttacher(wpID)
	if !files.empty() {
		if wpID == 0 {
			return nil, fmt.Errorf("cannot attach images: the comment does not belong to a work package")
		}
		texts, err := files.apply(ctx, t, a)
		if err != nil {
			return nil, err
		}
		body = texts["body"]
	}
	res, err := send(body)
	if err != nil {
		return nil, err
	}
	act := flattenActivity(res)
	e.nameUser(ctx, act, map[int]string{})
	if len(t.Uploaded) > 0 {
		act["uploaded"] = t.Uploaded
	}
	men.report(act)
	return act, nil
}

var wpActivities = &Op{
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
