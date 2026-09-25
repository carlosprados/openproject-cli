package ops

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() {
	registerGroup(&Group{
		Name:    "project",
		Aliases: []string{"projects", "proj"},
		Short:   "Projects: list, view, create, update, members, versions, categories",
		Long: `Projects can be referenced by numeric id, identifier (slug, e.g.
demo-project) or name (partial names work if unambiguous).`,
	})
	registerGroup(&Group{Name: "version", Aliases: []string{"versions", "milestone"}, Short: "Versions (releases / sprints) of projects"})
	registerGroup(&Group{Name: "membership", Aliases: []string{"memberships", "member"}, Short: "Project memberships: who has which role where"})

	register(projList, projGet, projCreate, projUpdate, projDelete, projAssignees, projCategories,
		verList, verCreate, verUpdate,
		memList, memCreate, memUpdate, memDelete)
}

var projectStatuses = []string{"not_started", "on_track", "at_risk", "off_track", "finished", "discontinued"}

var projList = &Op{
	Tool:    "op_list_projects",
	Cmd:     []string{"project", "list"},
	Aliases: []string{"ls"},
	Short:   "List projects",
	Example: `  opcli project list
  opcli project list --search radar
  opcli project list --archived`,
	Params: []Param{
		{Name: "search", Kind: String, Short: "q", Desc: "Filter by name or identifier (substring)"},
		{Name: "archived", Kind: Bool, Desc: "List archived projects instead of active ones"},
		{Name: "filter", Kind: String, Desc: "Extra raw API filters as a JSON array"},
		pMax(100), pPage,
	},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "IDENTIFIER", Key: "identifier"}, {Header: "NAME", Key: "name"}, {Header: "STATUS", Key: "status"}, {Header: "PARENT", Key: "parent"}, {Header: "PUBLIC", Key: "public"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		active := "t"
		if a.Bool("archived") {
			active = "f"
		}
		f := NewFilters().Add("active", "=", active)
		if s := a.String("search"); s != "" {
			f = f.Add("name_and_identifier", "~", s)
		}
		f, err := f.Merge(a.String("filter"))
		if err != nil {
			return nil, err
		}
		l, err := e.list(ctx, "/projects", url.Values{"filters": {f.JSON()}, "sortBy": {`[["name","asc"]]`}}, a)
		if err != nil {
			return nil, err
		}
		for _, p := range l.Items {
			trimProject(p)
		}
		return l, nil
	},
}

func trimProject(p map[string]any) {
	delete(p, "_type")
	delete(p, "storages")
	delete(p, "projectStorages")
	delete(p, "ancestors")
}

var projGet = &Op{
	Tool:    "op_get_project",
	Cmd:     []string{"project", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a project",
	Example: `  opcli project get demo-project
  opcli project get 3 -o json`,
	Params: []Param{{Name: "project", Kind: String, Required: true, Positional: true, Desc: "Project id, identifier or name"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Get(ctx, fmt.Sprintf("/projects/%d", pid), nil)
		if err != nil {
			return nil, err
		}
		p := hal.Flatten(res)
		trimProject(p)
		p["url"] = e.C.WebURL("/projects/" + fmt.Sprint(p["identifier"]))
		return p, nil
	},
}

var projFieldParams = []Param{
	{Name: "description", Kind: String, Short: "d", File: true, Desc: "Description (Markdown)"},
	{Name: "parent", Kind: String, Desc: "Parent project id/identifier/name ('none' for top level)"},
	{Name: "public", Kind: Bool, Desc: "Visible to everybody"},
	{Name: "status", Kind: String, Enum: projectStatuses, Desc: "Project status: " + strings.Join(projectStatuses, ", ")},
	{Name: "status_explanation", Kind: String, File: true, Desc: "Status explanation (Markdown)"},
}

var projCreate = &Op{
	Tool:  "op_create_project",
	Cmd:   []string{"project", "create"},
	Short: "Create a project",
	Long:  `Create a project. The identifier is derived from the name when omitted.`,
	Example: `  opcli project create --name "Website relaunch"
  opcli project create --name "Backend" --identifier web-backend --parent website-relaunch --public`,
	Params: append([]Param{
		{Name: "name", Kind: String, Required: true, Desc: "Project name"},
		{Name: "identifier", Kind: String, Desc: "URL identifier (lowercase, digits, dashes)"},
	}, projFieldParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		body, err := e.projectPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		body["name"] = a.String("name")
		ident := a.String("identifier")
		if ident == "" {
			ident = slugify(a.String("name"))
		}
		body["identifier"] = ident
		res, err := e.C.Post(ctx, "/projects", body)
		if err != nil {
			return nil, err
		}
		p := hal.Flatten(res)
		trimProject(p)
		return p, nil
	},
}

var projUpdate = &Op{
	Tool:  "op_update_project",
	Cmd:   []string{"project", "update"},
	Short: "Update a project (name, description, status, archive...)",
	Example: `  opcli project update demo-project --status at_risk --status-explanation "Vendor late"
  opcli project update 3 --name "New name"
  opcli project update old-stuff --archive`,
	Params: append([]Param{
		{Name: "project", Kind: String, Required: true, Positional: true, Desc: "Project id, identifier or name"},
		{Name: "name", Kind: String, Desc: "New name"},
		{Name: "identifier", Kind: String, Desc: "New identifier"},
		{Name: "archive", Kind: Bool, Desc: "Archive the project (true) or unarchive it (false)"},
	}, projFieldParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		body, err := e.projectPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		for _, k := range []string{"name", "identifier"} {
			if a.Has(k) {
				body[k] = a.String(k)
			}
		}
		if a.Has("archive") {
			body["active"] = !a.Bool("archive")
		}
		res, err := e.C.Patch(ctx, fmt.Sprintf("/projects/%d", pid), body)
		if err != nil {
			return nil, err
		}
		p := hal.Flatten(res)
		trimProject(p)
		return p, nil
	},
}

func (e *Env) projectPayload(ctx context.Context, a Args) (map[string]any, error) {
	body := map[string]any{}
	links := map[string]any{}
	if a.Has("description") {
		body["description"] = hal.Text(a.String("description"))
	}
	if a.Has("public") {
		body["public"] = a.Bool("public")
	}
	if a.Has("status_explanation") {
		body["statusExplanation"] = hal.Text(a.String("status_explanation"))
	}
	if a.Has("status") {
		links["status"] = hal.Ref("/api/v3/project_statuses/" + strings.ToLower(a.String("status")))
	}
	if a.Has("parent") {
		p := a.String("parent")
		if p == "" || strings.EqualFold(p, "none") {
			links["parent"] = hal.Ref("")
		} else {
			pid, err := e.ProjectID(ctx, p)
			if err != nil {
				return nil, err
			}
			links["parent"] = hal.Ref(href("projects", pid))
		}
	}
	if len(links) > 0 {
		body["_links"] = links
	}
	return body, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	repl := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n", "ç", "c")
	return strings.Trim(slugRe.ReplaceAllString(repl.Replace(strings.ToLower(s)), "-"), "-")
}

var projDelete = &Op{
	Tool:    "op_delete_project",
	Cmd:     []string{"project", "delete"},
	Short:   "Delete a project and everything in it (irreversible)",
	Example: `  opcli project delete old-stuff`,
	Params:  []Param{{Name: "project", Kind: String, Required: true, Positional: true, Desc: "Project id, identifier or name"}},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		if err := e.C.Delete(ctx, fmt.Sprintf("/projects/%d", pid)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("Project %d scheduled for deletion", pid)}, nil
	},
}

var projAssignees = &Op{
	Tool:    "op_list_project_assignees",
	Cmd:     []string{"project", "assignees"},
	Short:   "List users that can be assigned work packages in a project",
	Example: `  opcli project assignees demo-project`,
	Params:  []Param{pProjectArg()},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "LOGIN", Key: "login"}, {Header: "EMAIL", Key: "email"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		return e.list(ctx, fmt.Sprintf("/projects/%d/available_assignees", pid), nil, Args{})
	},
}

var projCategories = &Op{
	Tool:    "op_list_project_categories",
	Cmd:     []string{"project", "categories"},
	Short:   "List work package categories of a project",
	Example: `  opcli project categories demo-project`,
	Params:  []Param{pProjectArg()},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "DEFAULT ASSIGNEE", Key: "defaultAssignee"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		return e.list(ctx, fmt.Sprintf("/projects/%d/categories", pid), nil, Args{})
	},
}

var versionStatuses = []string{"open", "locked", "closed"}

var verList = &Op{
	Tool:    "op_list_versions",
	Cmd:     []string{"version", "list"},
	Aliases: []string{"ls"},
	Short:   "List versions of a project (or all visible versions with --project all)",
	Example: `  opcli version list -p demo-project
  opcli version list --project all`,
	Params:  []Param{pProject("Project id, identifier or name; 'all' for every project")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "STATUS", Key: "status"}, {Header: "START", Key: "startDate"}, {Header: "END", Key: "endDate"}, {Header: "SHARING", Key: "sharing"}, {Header: "PROJECT", Key: "definingProject"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		path := "/versions"
		if p := a.String("project"); !strings.EqualFold(p, "all") {
			pid, err := e.ProjectID(ctx, p)
			if err != nil {
				return nil, err
			}
			path = fmt.Sprintf("/projects/%d/versions", pid)
		}
		return e.list(ctx, path, nil, Args{})
	},
}

var verParams = []Param{
	{Name: "description", Kind: String, Short: "d", Desc: "Description"},
	{Name: "start_date", Kind: String, Desc: "Start date (YYYY-MM-DD, today, +7d...)"},
	{Name: "end_date", Kind: String, Desc: "Finish date (YYYY-MM-DD, +14d...)"},
	{Name: "status", Kind: String, Enum: versionStatuses, Desc: "open, locked or closed"},
	{Name: "sharing", Kind: String, Enum: []string{"none", "descendants", "hierarchy", "tree", "system"}, Desc: "Share with: none, descendants, hierarchy, tree, system"},
}

var verCreate = &Op{
	Tool:    "op_create_version",
	Cmd:     []string{"version", "create"},
	Short:   "Create a version in a project",
	Example: `  opcli version create -p demo-project --name "v1.2" --end-date 2026-11-30`,
	Params: append([]Param{
		pProject(""),
		{Name: "name", Kind: String, Required: true, Desc: "Version name"},
	}, verParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		body, err := versionPayload(a)
		if err != nil {
			return nil, err
		}
		body["name"] = a.String("name")
		body["_links"] = map[string]any{"definingProject": hal.Ref(href("projects", pid))}
		res, err := e.C.Post(ctx, "/versions", body)
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

var verUpdate = &Op{
	Tool:    "op_update_version",
	Cmd:     []string{"version", "update"},
	Short:   "Update a version (dates, status, name...)",
	Example: `  opcli version update 4 --status closed`,
	Params:  append([]Param{pID("Version"), {Name: "name", Kind: String, Desc: "New name"}}, verParams...),
	Safety:  Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		body, err := versionPayload(a)
		if err != nil {
			return nil, err
		}
		if a.Has("name") {
			body["name"] = a.String("name")
		}
		res, err := e.C.Patch(ctx, "/versions/"+url.PathEscape(a.String("id")), body)
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

func versionPayload(a Args) (map[string]any, error) {
	body := map[string]any{}
	if a.Has("description") {
		body["description"] = hal.Text(a.String("description"))
	}
	for param, field := range map[string]string{"start_date": "startDate", "end_date": "endDate"} {
		if a.Has(param) {
			d, err := dateOrClear(a, param)
			if err != nil {
				return nil, err
			}
			body[field] = d
		}
	}
	for _, k := range []string{"status", "sharing"} {
		if a.Has(k) {
			body[k] = strings.ToLower(a.String(k))
		}
	}
	return body, nil
}

var memColumns = []Column{{Header: "ID", Key: "id"}, {Header: "PROJECT", Key: "project"}, {Header: "PRINCIPAL", Key: "principal"}, {Header: "ROLES", Key: "roles"}}

var memList = &Op{
	Tool:    "op_list_memberships",
	Cmd:     []string{"membership", "list"},
	Aliases: []string{"ls"},
	Short:   "List memberships, optionally by project and/or user",
	Example: `  opcli membership list -p demo-project
  opcli membership list --user me`,
	Params: []Param{
		pProject("Project id, identifier or name (default: all projects)"),
		{Name: "user", Kind: String, Desc: "me, or login/email/name/id"},
		pMax(200),
	},
	Columns: memColumns,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		f := NewFilters()
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			f = f.Add("project", "=", pid)
		}
		if a.Has("user") {
			uid, err := e.UserID(ctx, a.String("user"))
			if err != nil {
				return nil, err
			}
			f = f.Add("principal", "=", uid)
		}
		return e.list(ctx, "/memberships", url.Values{"filters": {f.JSON()}}, a)
	},
}

var memCreate = &Op{
	Tool:    "op_create_membership",
	Cmd:     []string{"membership", "add"},
	Aliases: []string{"create"},
	Short:   "Add a user or group to a project with one or more roles",
	Example: `  opcli membership add -p demo-project --user jane.doe --roles Member
  opcli membership add -p demo-project --user devs --roles "Member,Reader" --message "Welcome!"`,
	Params: []Param{
		pProject(""),
		{Name: "user", Kind: String, Required: true, Desc: "User/group: login, email, name or id"},
		{Name: "roles", Kind: Strings, Required: true, Desc: "Role names or ids (see 'opcli role list')"},
		{Name: "message", Kind: String, Desc: "Custom text for the notification email"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		uid, err := e.UserID(ctx, a.String("user"))
		if err != nil {
			return nil, err
		}
		roles, err := e.roleRefs(ctx, a.Strings("roles"))
		if err != nil {
			return nil, err
		}
		body := map[string]any{"_links": map[string]any{
			"project": hal.Ref(href("projects", pid)), "principal": hal.Ref(href("principals", uid)), "roles": roles,
		}}
		if m := a.String("message"); m != "" {
			body["_meta"] = map[string]any{"notificationMessage": hal.Text(m), "sendNotifications": true}
		}
		res, err := e.C.Post(ctx, "/memberships", body)
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

var memUpdate = &Op{
	Tool:    "op_update_membership",
	Cmd:     []string{"membership", "update"},
	Short:   "Replace the roles of a membership",
	Example: `  opcli membership update 12 --roles "Project admin"`,
	Params: []Param{
		pID("Membership"),
		{Name: "roles", Kind: Strings, Required: true, Desc: "Role names or ids"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		roles, err := e.roleRefs(ctx, a.Strings("roles"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Patch(ctx, "/memberships/"+url.PathEscape(a.String("id")), map[string]any{"_links": map[string]any{"roles": roles}})
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

func (e *Env) roleRefs(ctx context.Context, refs []string) ([]any, error) {
	var out []any
	for _, r := range refs {
		id, err := e.RoleID(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, hal.Ref(href("roles", id)))
	}
	return out, nil
}

var memDelete = &Op{
	Tool:    "op_delete_membership",
	Cmd:     []string{"membership", "delete"},
	Aliases: []string{"rm"},
	Short:   "Remove a membership (the user loses access to the project)",
	Example: `  opcli membership delete 12`,
	Params:  []Param{pID("Membership")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if err := e.C.Delete(ctx, "/memberships/"+url.PathEscape(a.String("id"))); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted membership " + a.String("id")}, nil
	},
}
