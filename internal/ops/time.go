package ops

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() {
	registerGroup(&Group{
		Name:    "time",
		Aliases: []string{"te", "time-entry", "timelog"},
		Short:   "Time tracking: log, list, update and delete time entries",
		Long: `Time entries record hours spent on a work package.

Hours accept 1.5, 1,5, 90m, 1h30m or PT1H30M. Dates accept YYYY-MM-DD,
today, yesterday, week (this Monday), month (1st of this month) or -Nd.`,
	})
	register(timeList, timeLog, timeUpdate, timeDelete, timeActivities)
}

// TimeList is a List of time entries plus their hour total.
type TimeList struct {
	List
	TotalHours float64 `json:"totalHours"`
}

var (
	timeColumns = []Column{
		{Header: "ID", Key: "id"}, {Header: "DATE", Key: "spentOn"}, {Header: "HOURS", Key: "hours", Fmt: hoursFmt},
		{Header: "USER", Key: "user"}, {Header: "PROJECT", Key: "project"}, {Header: "WP", Key: "entityId"},
		{Header: "WORK PACKAGE", Key: "entity"}, {Header: "ACTIVITY", Key: "activity"}, {Header: "COMMENT", Key: "comment"},
	}
	groupColumns = []Column{{Header: "GROUP", Key: "group"}, {Header: "HOURS", Key: "hours"}, {Header: "ENTRIES", Key: "entries"}}
)

var timeList = &Op{
	Tool:    "op_list_time_entries",
	Cmd:     []string{"time", "list"},
	Aliases: []string{"ls"},
	Short:   "List time entries (yours by default) with an hour total",
	Example: `  opcli time list                          # my entries, this week
  opcli time list --from month             # my entries, this month
  opcli time list --from 2026-09-01 --to 2026-09-30 --user all -p demo-project
  opcli time list --wp 42 --user all --from -365d
  opcli time list --group-by day`,
	Params: []Param{
		{Name: "user", Kind: String, Short: "u", Default: "me", Desc: "me (default), all, or login/email/name/id"},
		pProject("Project id, identifier or name (default: all projects)"),
		{Name: "wp", Kind: String, Desc: "Only entries of this work package id"},
		{Name: "from", Kind: String, Default: "week", Desc: "First day (inclusive): YYYY-MM-DD, today, yesterday, week, month, -Nd"},
		{Name: "to", Kind: String, Default: "today", Desc: "Last day (inclusive)"},
		{Name: "group_by", Kind: String, Enum: []string{"day", "project", "wp", "activity", "user"}, Desc: "Summarise hours by day, project, wp, activity or user instead of listing entries"},
		pMax(500),
	},
	Columns: timeColumns,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		f := NewFilters()
		if u := a.String("user"); !strings.EqualFold(u, "all") && u != "" {
			var err error
			if f, err = e.userFilter(ctx, f, "user", u); err != nil {
				return nil, err
			}
		}
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			f = f.Add("project_id", "=", pid)
		}
		if a.Has("wp") {
			id, err := WorkPackageID(a.String("wp"))
			if err != nil {
				return nil, err
			}
			f = f.Add("entity_type", "=", "WorkPackage").Add("entity_id", "=", id)
		}
		from, err := ParseDate(a.String("from"))
		if err != nil {
			return nil, err
		}
		to, err := ParseDate(a.String("to"))
		if err != nil {
			return nil, err
		}
		if from != "" || to != "" {
			f = f.Add("spent_on", "<>d", from, to)
		}
		l, err := e.list(ctx, "/time_entries", url.Values{"filters": {f.JSON()}, "sortBy": {`[["spent_on","desc"]]`}}, a)
		if err != nil {
			return nil, err
		}
		total := 0.0
		for _, it := range l.Items {
			delete(it, "_type")
			h, _ := hal.ParseHours(fmt.Sprint(it["hours"]))
			total += h
		}
		tl := &TimeList{List: *l, TotalHours: round2(total)}
		if g := a.String("group_by"); g != "" {
			tl.Items = groupHours(l.Items, g)
			tl.Count = len(tl.Items)
		}
		return tl, nil
	},
	Text: func(w io.Writer, v any) error {
		tl := v.(*TimeList)
		cols := timeColumns
		if len(tl.Items) > 0 && tl.Items[0]["group"] != nil {
			cols = groupColumns
		}
		if err := renderTable(w, &tl.List, cols); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "\nTotal: %sh in %d entries\n", hal.String(tl.TotalHours), tl.Total)
		return err
	},
}

func groupHours(items []map[string]any, by string) []map[string]any {
	key := map[string]string{"day": "spentOn", "project": "project", "wp": "entity", "activity": "activity", "user": "user"}[by]
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, it := range items {
		k := hal.String(it[key])
		if by == "wp" {
			k = fmt.Sprintf("#%s %s", hal.String(it["entityId"]), k)
		}
		h, _ := hal.ParseHours(fmt.Sprint(it["hours"]))
		sums[k] += h
		counts[k]++
	}
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"group": k, "hours": round2(sums[k]), "entries": counts[k]})
	}
	return out
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

var timeLog = &Op{
	Tool:    "op_log_time",
	Cmd:     []string{"time", "log"},
	Aliases: []string{"add", "create"},
	Short:   "Log time on a work package",
	Long: `Log hours on a work package. The project is taken from the work package.
--activity is required by some instances; list valid ones with
'opcli time activities --wp <id>'.`,
	Example: `  opcli time log --wp 42 --hours 1.5 --comment "Code review"
  opcli time log --wp 42 --hours 2h --date yesterday --activity Development
  opcli time log --wp 42 --hours 45m --user jane.doe      # admins: on behalf of others`,
	Params: []Param{
		{Name: "wp", Kind: String, Required: true, Desc: "Work package id"},
		{Name: "hours", Kind: String, Required: true, Short: "H", Desc: "Time spent: 1.5, 90m, 1h30m, PT1H30M"},
		{Name: "date", Kind: String, Default: "today", Desc: "Day the work was done (YYYY-MM-DD, today, yesterday, -Nd)"},
		{Name: "comment", Kind: String, Short: "m", Desc: "What was done"},
		{Name: "activity", Kind: String, Desc: "Activity name or id (Development, Management, ...)"},
		{Name: "user", Kind: String, Desc: "Log on behalf of this user (needs permission)"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		wp, err := WorkPackageID(a.String("wp"))
		if err != nil {
			return nil, err
		}
		body, err := e.timePayload(ctx, a, wp)
		if err != nil {
			return nil, err
		}
		body["_links"].(map[string]any)["entity"] = hal.Ref(href("work_packages", wp))
		res, err := e.C.Post(ctx, "/time_entries", body)
		if err != nil {
			return nil, err
		}
		te := hal.Flatten(res)
		delete(te, "_type")
		return te, nil
	},
}

var timeUpdate = &Op{
	Tool:    "op_update_time_entry",
	Cmd:     []string{"time", "update"},
	Aliases: []string{"edit"},
	Short:   "Update a time entry",
	Example: `  opcli time update 17 --hours 3 --comment "Longer than expected"
  opcli time update 17 --date 2026-09-24 --wp 43`,
	Params: []Param{
		pID("Time entry"),
		{Name: "wp", Kind: String, Desc: "Move to this work package id"},
		{Name: "hours", Kind: String, Short: "H", Desc: "Time spent: 1.5, 90m, 1h30m, PT1H30M"},
		{Name: "date", Kind: String, Desc: "Day the work was done"},
		{Name: "comment", Kind: String, Short: "m", Desc: "What was done"},
		{Name: "activity", Kind: String, Desc: "Activity name or id"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := a.String("id")
		wp := 0
		if a.Has("wp") {
			var err error
			if wp, err = WorkPackageID(a.String("wp")); err != nil {
				return nil, err
			}
		} else if a.Has("activity") {
			cur, err := e.C.Get(ctx, "/time_entries/"+url.PathEscape(id), nil)
			if err != nil {
				return nil, err
			}
			wp = intOf(hal.IDFromHref(hal.Link(cur, "entity")))
		}
		body, err := e.timePayload(ctx, a, wp)
		if err != nil {
			return nil, err
		}
		if a.Has("wp") {
			body["_links"].(map[string]any)["entity"] = hal.Ref(href("work_packages", wp))
		}
		res, err := e.C.Patch(ctx, "/time_entries/"+url.PathEscape(id), body)
		if err != nil {
			return nil, err
		}
		te := hal.Flatten(res)
		delete(te, "_type")
		return te, nil
	},
}

func (e *Env) timePayload(ctx context.Context, a Args, wp int) (map[string]any, error) {
	links := map[string]any{}
	body := map[string]any{"_links": links}
	if a.Has("hours") {
		h, err := hal.ParseHours(a.String("hours"))
		if err != nil {
			return nil, err
		}
		if h <= 0 {
			return nil, fmt.Errorf("hours must be positive")
		}
		body["hours"] = hal.ISOHours(h)
	}
	if a.Has("date") {
		d, err := ParseDate(a.String("date"))
		if err != nil {
			return nil, err
		}
		body["spentOn"] = d
	}
	if a.Has("comment") {
		body["comment"] = hal.Text(a.String("comment"))
	}
	if a.Has("activity") {
		id, err := e.ActivityID(ctx, wp, a.String("activity"))
		if err != nil {
			return nil, err
		}
		links["activity"] = hal.Ref(fmt.Sprintf("/api/v3/time_entries/activities/%d", id))
	}
	if a.Has("user") {
		h, err := e.UserHref(ctx, a.String("user"))
		if err != nil {
			return nil, err
		}
		links["user"] = hal.Ref(h)
	}
	return body, nil
}

var timeDelete = &Op{
	Tool:    "op_delete_time_entry",
	Cmd:     []string{"time", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete a time entry",
	Example: `  opcli time delete 17`,
	Params:  []Param{pID("Time entry")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if err := e.C.Delete(ctx, "/time_entries/"+url.PathEscape(a.String("id"))); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted time entry " + a.String("id")}, nil
	},
}

var timeActivities = &Op{
	Tool:  "op_list_time_activities",
	Cmd:   []string{"time", "activities"},
	Short: "List activities valid for time entries (optionally for a work package)",
	Example: `  opcli time activities
  opcli time activities --wp 42`,
	Params:  []Param{{Name: "wp", Kind: String, Desc: "Work package id (activities can be restricted per project)"}},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		wp := 0
		if a.Has("wp") {
			var err error
			if wp, err = WorkPackageID(a.String("wp")); err != nil {
				return nil, err
			}
		}
		acts, err := e.Activities(ctx, wp)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, len(acts))
		for i, it := range acts {
			items[i] = map[string]any{"id": it.ID, "name": it.Name}
		}
		return NewList(items, len(items)), nil
	},
}
