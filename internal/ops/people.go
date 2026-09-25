package ops

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() {
	registerGroup(&Group{Name: "user", Aliases: []string{"users", "principal"}, Short: "Users, groups and placeholder users"})
	registerGroup(&Group{Name: "notification", Aliases: []string{"notifications", "inbox", "notif"}, Short: "Your in-app notifications (mentions, assignments, changes)"})
	register(whoami, userList, userGet, notifList, notifRead)
}

var userColumns = []Column{{Header: "ID", Key: "id"}, {Header: "TYPE", Key: "_type"}, {Header: "NAME", Key: "name"}, {Header: "LOGIN", Key: "login"}, {Header: "EMAIL", Key: "email"}, {Header: "STATUS", Key: "status"}}

var whoami = &Op{
	Tool:    "op_whoami",
	Cmd:     []string{"whoami"},
	Aliases: []string{"me"},
	Short:   "Show the authenticated user (checks URL and API key)",
	Example: `  opcli whoami
  opcli whoami -o json`,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		res, err := e.C.Get(ctx, "/users/me", nil)
		if err != nil {
			return nil, err
		}
		u := hal.Flatten(res)
		u["server"] = e.C.BaseURL
		return u, nil
	},
}

var userList = &Op{
	Tool:    "op_list_users",
	Cmd:     []string{"user", "list"},
	Aliases: []string{"ls", "search"},
	Short:   "List or search users, groups and placeholder users",
	Example: `  opcli user list
  opcli user list --search jane
  opcli user list --kind group
  opcli user list -p demo-project           # members of a project`,
	Params: []Param{
		{Name: "search", Kind: String, Short: "q", Desc: "Match name, login or email (substring)"},
		{Name: "kind", Kind: String, Default: "user", Enum: []string{"user", "group", "placeholder", "all"}, Desc: "user (default), group, placeholder or all"},
		pProject("Only members of this project"),
		pMax(100), pPage,
	},
	Columns: userColumns,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		f := NewFilters()
		switch strings.ToLower(a.String("kind")) {
		case "user":
			f = f.Add("type", "=", "User")
		case "group":
			f = f.Add("type", "=", "Group")
		case "placeholder":
			f = f.Add("type", "=", "PlaceholderUser")
		}
		if s := a.String("search"); s != "" {
			f = f.Add("any_name_attribute", "~", s)
		}
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			f = f.Add("member", "=", pid)
		}
		return e.list(ctx, "/principals", url.Values{"filters": {f.JSON()}, "sortBy": {`[["name","asc"]]`}}, a)
	},
}

var userGet = &Op{
	Tool:    "op_get_user",
	Cmd:     []string{"user", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a user",
	Example: `  opcli user get me
  opcli user get jane.doe
  opcli user get 8 -o json`,
	Params: []Param{{Name: "user", Kind: String, Required: true, Positional: true, Desc: "me, or login/email/name/id"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := e.UserID(ctx, a.String("user"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Get(ctx, fmt.Sprintf("/users/%d", id), nil)
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

var notifList = &Op{
	Tool:    "op_list_notifications",
	Cmd:     []string{"notification", "list"},
	Aliases: []string{"ls"},
	Short:   "List your notifications (unread by default)",
	Example: `  opcli notification list
  opcli notification list --all --max 100
  opcli notification list --reason mentioned`,
	Params: []Param{
		{Name: "all", Kind: Bool, Desc: "Include already read notifications"},
		{Name: "reason", Kind: String, Desc: "Only this reason: mentioned, assigned, responsible, watched, commented, created, processed, prioritized, scheduled, dateAlert, shared"},
		pProject("Only notifications of this project"),
		pMax(50), pPage,
	},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "DATE", Key: "createdAt"}, {Header: "REASON", Key: "reason"}, {Header: "ACTOR", Key: "actor"}, {Header: "PROJECT", Key: "project"}, {Header: "RESOURCE", Key: "resource"}, {Header: "READ", Key: "readIAN"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		f := NewFilters()
		if !a.Bool("all") {
			f = f.Add("readIAN", "=", "f")
		}
		if r := a.String("reason"); r != "" {
			f = f.Add("reason", "=", r)
		}
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			f = f.Add("project", "=", pid)
		}
		l, err := e.list(ctx, "/notifications", url.Values{"filters": {f.JSON()}}, a)
		if err != nil {
			return nil, err
		}
		for _, it := range l.Items {
			delete(it, "_type")
			delete(it, "details")
		}
		return l, nil
	},
}

var notifRead = &Op{
	Tool:  "op_mark_notifications_read",
	Cmd:   []string{"notification", "read"},
	Short: "Mark one notification (by id) or all unread notifications as read",
	Example: `  opcli notification read 123
  opcli notification read --all`,
	Params: []Param{
		{Name: "id", Kind: String, Positional: true, Desc: "Notification id (omit with --all)"},
		{Name: "all", Kind: Bool, Desc: "Mark every unread notification as read"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		switch {
		case a.String("id") != "":
			if _, err := e.C.Post(ctx, "/notifications/"+url.PathEscape(a.String("id"))+"/read_ian", nil); err != nil {
				return nil, err
			}
			return &Message{OK: true, Message: "Marked notification " + a.String("id") + " as read"}, nil
		case a.Bool("all"):
			f := NewFilters().Add("readIAN", "=", "f")
			if _, err := e.C.JSON(ctx, "POST", "/notifications/read_ian", url.Values{"filters": {f.JSON()}}, nil); err != nil {
				return nil, err
			}
			return &Message{OK: true, Message: "Marked all notifications as read"}, nil
		}
		return nil, fmt.Errorf("give a notification id or --all")
	},
}
