package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Reusable parameter declarations, so names and wording stay uniform.
func pMax(def int) Param {
	return Param{Name: "max", Kind: Int, Default: def, Desc: "Maximum number of results (0 = all)"}
}

var pPage = Param{Name: "page", Kind: Int, Desc: "Fetch only this page (1-based) of size --max instead of paging automatically"}

func pProject(desc string) Param {
	if desc == "" {
		desc = "Project id, identifier or name (default: configured project)"
	}
	return Param{Name: "project", Kind: String, Short: "p", Desc: desc}
}

// pProjectArg is pProject taken as an optional positional argument.
func pProjectArg() Param {
	p := pProject("Project id, identifier or name (default: configured project)")
	p.Positional = true
	p.Short = ""
	return p
}

func pID(what string) Param {
	return Param{Name: "id", Kind: String, Required: true, Positional: true, Desc: what + " id"}
}

// list fetches a collection honouring the max/page params and flattens it.
func (e *Env) list(ctx context.Context, path string, q url.Values, a Args, keepEmbedded ...string) (*List, error) {
	if q == nil {
		q = url.Values{}
	}
	max := a.Int("max")
	if page := a.Int("page"); page > 0 {
		size := max
		if size <= 0 {
			size = 100
		}
		q.Set("pageSize", strconv.Itoa(size))
		q.Set("offset", strconv.Itoa(page))
		res, err := e.C.Get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		return NewList(hal.FlattenAll(client.Elements(res), keepEmbedded...), intOf(res["total"])), nil
	}
	items, total, err := e.C.Collect(ctx, path, q, max)
	if err != nil {
		return nil, err
	}
	return NewList(hal.FlattenAll(items, keepEmbedded...), total), nil
}

// sortBy converts "updatedAt:desc,id" into the API sortBy JSON.
func sortBy(spec string) (string, error) {
	var out [][2]string
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		field, dir, _ := strings.Cut(part, ":")
		if dir == "" {
			dir = "asc"
		}
		dir = strings.ToLower(dir)
		if dir != "asc" && dir != "desc" {
			return "", fmt.Errorf("invalid sort direction %q (asc|desc)", dir)
		}
		out = append(out, [2]string{field, dir})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ParseDate accepts YYYY-MM-DD, today, yesterday, tomorrow, week (Monday
// of the current week), month (first day of the month) and relative day
// offsets such as -7d or +3d. Empty input yields "".
func ParseDate(s string) (string, error) {
	return parseDateAt(s, time.Now())
}

func parseDateAt(s string, now time.Time) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch s {
	case "":
		return "", nil
	case "today":
		return day.Format(time.DateOnly), nil
	case "yesterday":
		return day.AddDate(0, 0, -1).Format(time.DateOnly), nil
	case "tomorrow":
		return day.AddDate(0, 0, 1).Format(time.DateOnly), nil
	case "week":
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset).Format(time.DateOnly), nil
	case "month":
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location()).Format(time.DateOnly), nil
	}
	if strings.HasSuffix(s, "d") && (s[0] == '-' || s[0] == '+') {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil {
			return day.AddDate(0, 0, n).Format(time.DateOnly), nil
		}
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return "", fmt.Errorf("invalid date %q (use YYYY-MM-DD, today, yesterday, week, month or -Nd)", s)
	}
	return t.Format(time.DateOnly), nil
}

// dateOrClear resolves a date param where "none" or "" clears the field.
func dateOrClear(a Args, name string) (any, error) {
	v := a.String(name)
	if v == "" || strings.EqualFold(v, "none") {
		return nil, nil
	}
	return ParseDate(v)
}

func href(kind string, id int) string {
	return fmt.Sprintf("/api/v3/%s/%d", kind, id)
}
