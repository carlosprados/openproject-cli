package ops

import (
	"context"
	"fmt"
	"net/url"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Reference data (statuses, types, priorities, roles), saved queries and
// meetings.

func init() {
	registerGroup(&Group{Name: "status", Aliases: []string{"statuses"}, Short: "Work package statuses"})
	registerGroup(&Group{Name: "type", Aliases: []string{"types"}, Short: "Work package types"})
	registerGroup(&Group{Name: "priority", Aliases: []string{"priorities"}, Short: "Work package priorities"})
	registerGroup(&Group{Name: "role", Aliases: []string{"roles"}, Short: "Roles for project memberships"})
	registerGroup(&Group{Name: "query", Aliases: []string{"queries", "view"}, Short: "Saved work package queries (views)"})
	registerGroup(&Group{Name: "meeting", Aliases: []string{"meetings"}, Short: "Meetings"})
	register(statusList, typeList, priorityList, roleList, queryList, meetingList, meetingGet)
}

var statusList = &Op{
	Tool:    "op_list_statuses",
	Cmd:     []string{"status", "list"},
	Aliases: []string{"ls"},
	Short:   "List all work package statuses",
	Long:    `List every status. Which ones a given work package may move to depends on the workflow: use 'opcli wp allowed <id> --field status'.`,
	Example: `  opcli status list`,
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "CLOSED", Key: "isClosed"}, {Header: "DEFAULT", Key: "isDefault"}, {Header: "READONLY", Key: "isReadonly"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		return e.list(ctx, "/statuses", nil, Args{})
	},
}

var typeList = &Op{
	Tool:    "op_list_types",
	Cmd:     []string{"type", "list"},
	Aliases: []string{"ls"},
	Short:   "List work package types, globally or enabled in a project",
	Example: `  opcli type list
  opcli type list -p demo-project`,
	Params:  []Param{pProject("Only types enabled in this project (default: all types)")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "MILESTONE", Key: "isMilestone"}, {Header: "DEFAULT", Key: "isDefault"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		path := "/types"
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			path = fmt.Sprintf("/projects/%d/types", pid)
		}
		return e.list(ctx, path, nil, Args{})
	},
}

var priorityList = &Op{
	Tool:    "op_list_priorities",
	Cmd:     []string{"priority", "list"},
	Aliases: []string{"ls"},
	Short:   "List work package priorities",
	Example: `  opcli priority list`,
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "DEFAULT", Key: "isDefault"}, {Header: "ACTIVE", Key: "isActive"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		return e.list(ctx, "/priorities", nil, Args{})
	},
}

var roleList = &Op{
	Tool:    "op_list_roles",
	Cmd:     []string{"role", "list"},
	Aliases: []string{"ls"},
	Short:   "List roles",
	Example: `  opcli role list`,
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		return e.list(ctx, "/roles", nil, Args{})
	},
}

var queryList = &Op{
	Tool:    "op_list_queries",
	Cmd:     []string{"query", "list"},
	Aliases: []string{"ls"},
	Short:   "List saved work package queries; run one with 'opcli wp list --query-id <id>'",
	Example: `  opcli query list
  opcli query list -p demo-project
  opcli wp list --query-id 5`,
	Params:  []Param{pProject("Only queries of this project (default: all visible)")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "PROJECT", Key: "project"}, {Header: "PUBLIC", Key: "public"}, {Header: "STARRED", Key: "starred"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		q := url.Values{}
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			q.Set("filters", NewFilters().Add("project", "=", pid).JSON())
		}
		l, err := e.list(ctx, "/queries", q, Args{})
		if err != nil {
			return nil, err
		}
		keep := map[string]bool{"id": true, "name": true, "project": true, "projectId": true, "public": true,
			"starred": true, "hidden": true, "createdAt": true, "updatedAt": true, "user": true, "userId": true}
		for _, it := range l.Items {
			for k := range it {
				if !keep[k] {
					delete(it, k)
				}
			}
		}
		return l, nil
	},
}

var meetingList = &Op{
	Tool:    "op_list_meetings",
	Cmd:     []string{"meeting", "list"},
	Aliases: []string{"ls"},
	Short:   "List meetings",
	Example: `  opcli meeting list
  opcli meeting list -p demo-project`,
	Params: []Param{
		pProject("Only meetings of this project (default: all visible)"),
		pMax(50), pPage,
	},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "TITLE", Key: "title"}, {Header: "START", Key: "startTime"}, {Header: "DURATION", Key: "duration"}, {Header: "LOCATION", Key: "location"}, {Header: "PROJECT", Key: "project"}, {Header: "STATE", Key: "state"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		q := url.Values{}
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			q.Set("filters", NewFilters().Add("project_id", "=", pid).JSON())
		}
		return e.list(ctx, "/meetings", q, a)
	},
}

var meetingGet = &Op{
	Tool:    "op_get_meeting",
	Cmd:     []string{"meeting", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a meeting with its agenda items",
	Example: `  opcli meeting get 3`,
	Params:  []Param{pID("Meeting")},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := url.PathEscape(a.String("id"))
		res, err := e.C.Get(ctx, "/meetings/"+id, nil)
		if err != nil {
			return nil, err
		}
		m := hal.Flatten(res)
		items, _, err := e.C.Collect(ctx, "/meetings/"+id+"/agenda_items", nil, 0)
		if err == nil {
			m["agendaItems"] = hal.FlattenAll(items)
		}
		return m, nil
	},
}
