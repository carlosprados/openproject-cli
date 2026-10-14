package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

type statusInfo struct {
	Name     string
	Closed   bool
	Default  bool
	Position int
}

// category is a todo/doing/done reading of OpenProject statuses, which
// carry no category: closed ones are done, the default (new) one is todo,
// every other is doing.
func (s statusInfo) category() string {
	switch {
	case s.Closed:
		return "done"
	case s.Default:
		return "todo"
	}
	return "doing"
}

// statusInfos is the status catalogue by id (cached; empty after a warned
// failure).
func (e *Env) statusInfos(ctx context.Context) map[int]statusInfo {
	e.cache.mu.Lock()
	st := e.cache.statuses
	e.cache.mu.Unlock()
	if st != nil {
		return st
	}
	st = map[int]statusInfo{}
	items, _, err := e.C.Collect(ctx, "/statuses", nil, 0)
	if err != nil {
		e.warnf("cannot read the statuses (%v): no statusIsClosed/statusCategory", err)
	}
	for _, it := range items {
		st[intOf(it["id"])] = statusInfo{
			Name: hal.String(it["name"]), Closed: it["isClosed"] == true,
			Default: it["isDefault"] == true, Position: intOf(it["position"]),
		}
	}
	e.cache.mu.Lock()
	e.cache.statuses = st
	e.cache.mu.Unlock()
	return st
}

func (e *Env) statusByName(ctx context.Context) map[string]statusInfo {
	out := map[string]statusInfo{}
	for _, s := range e.statusInfos(ctx) {
		out[s.Name] = s
	}
	return out
}

// decorateStatus adds statusIsClosed and statusCategory to a flattened WP.
func (e *Env) decorateStatus(ctx context.Context, it map[string]any) {
	if s, ok := e.statusInfos(ctx)[intOf(it["statusId"])]; ok {
		it["statusIsClosed"] = s.Closed
		it["statusCategory"] = s.category()
	}
}

// forEach runs fn(0..n-1), client.CollectParallel at a time.
func forEach(n int, fn func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, client.CollectParallel)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

// wpPageFallback recovers a page of work packages the server failed to
// render (5xx). The usual cause is one type's schema, which collections
// embed: a form configuration pointing at a deleted query breaks every page
// holding that type. The page's ids come back through the lightweight
// select path, are re-read grouped by project and type, and a group that
// still fails is read one by one; what fails alone is skipped. All of it is
// reported on stderr, never silently.
func (e *Env) wpPageFallback(path string) client.PageFallback {
	return func(ctx context.Context, q url.Values, cause error) ([]map[string]any, int, error) {
		var apiErr *client.APIError
		if !errors.As(cause, &apiErr) || apiErr.Status < 500 {
			return nil, 0, cause
		}
		iq := url.Values{}
		for k, v := range q {
			iq[k] = v
		}
		iq.Set("select", "total,elements/id,elements/project,elements/type")
		res, err := e.C.Get(ctx, path, iq)
		if err != nil {
			return nil, 0, fmt.Errorf("%w (reading the page's ids failed too: %v)", cause, err)
		}
		var ids []int
		groups := map[string][]int{}
		var order []string
		for _, el := range client.Elements(res) {
			id := intOf(el["id"])
			g := fmt.Sprintf("%v-%v", hal.IDFromHref(hal.Link(el, "project")), hal.IDFromHref(hal.Link(el, "type")))
			if groups[g] == nil {
				order = append(order, g)
			}
			groups[g] = append(groups[g], id)
			ids = append(ids, id)
		}

		got := map[int]map[string]any{}
		var broken []string
		var single []int
		for _, g := range order {
			items, err := e.wpsByID(ctx, groups[g])
			if err != nil {
				broken = append(broken, g)
				single = append(single, groups[g]...)
				continue
			}
			for _, it := range items {
				got[intOf(it["id"])] = it
			}
		}
		fetched := make([]map[string]any, len(single))
		forEach(len(single), func(i int) {
			fetched[i], _ = e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", single[i]), nil)
		})
		var skipped []string
		for i, id := range single {
			if fetched[i] == nil {
				skipped = append(skipped, "#"+strconv.Itoa(id))
				continue
			}
			got[id] = fetched[i]
		}

		out := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			if it, ok := got[id]; ok {
				out = append(out, it)
			}
		}
		msg := fmt.Sprintf("work package page %s failed (%v); re-read it by id: %d of %d recovered", q.Get("offset"), cause, len(out), len(ids))
		if len(broken) > 0 {
			msg += fmt.Sprintf("; project-type %s only one by one (check those types' form configuration)", strings.Join(broken, ", "))
		}
		if len(skipped) > 0 {
			msg += "; skipped " + strings.Join(skipped, ", ")
		}
		e.warnf("%s", msg)
		return out, intOf(res["total"]), nil
	}
}

// wpsByID reads the given work packages in one collection request.
func (e *Env) wpsByID(ctx context.Context, ids []int) ([]map[string]any, error) {
	vals := make([]any, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	q := url.Values{
		"filters":  {NewFilters().Add("id", "=", vals...).JSON()},
		"pageSize": {strconv.Itoa(len(ids))},
		"offset":   {"1"},
	}
	res, err := e.C.Get(ctx, "/work_packages", q)
	if err != nil {
		return nil, err
	}
	return client.Elements(res), nil
}

// Status history from activities. Activity details are rendered in the
// user's language ("Status changed from A to B", "Estado se cambió de A a
// B"), so a status change is recognised by its label — the status field's
// localised name, taken from a work package schema — and by its values
// being status names, never by the English wording.

var (
	reDetailLabel = regexp.MustCompile(`<strong>(.*?)</strong>`)
	reDetailValue = regexp.MustCompile(`<i>(.*?)</i>`)
)

type statusSpan struct {
	Status  string
	Entered time.Time
	Left    time.Time // zero while it is the current status
}

type wpHistory struct {
	WP      map[string]any
	Spans   []statusSpan
	Created time.Time
	Closed  time.Time // last entry into the final run of closed statuses
}

// statusChange reads one activity detail (its html form).
func statusChange(detailHTML, label string, known map[string]statusInfo) (from, to string, ok bool) {
	m := reDetailLabel.FindStringSubmatch(detailHTML)
	if m == nil {
		return "", "", false
	}
	if label != "" && html.UnescapeString(m[1]) != label {
		return "", "", false
	}
	var vals []string
	for _, v := range reDetailValue.FindAllStringSubmatch(detailHTML, -1) {
		s := html.UnescapeString(v[1])
		if _, ok := known[s]; !ok {
			return "", "", false
		}
		vals = append(vals, s)
	}
	switch len(vals) {
	case 1:
		return "", vals[0], true
	case 2:
		return vals[0], vals[1], true
	}
	return "", "", false
}

// statusLabel finds the localised status field name in the schema of one
// of the given work packages' project and type.
func (e *Env) statusLabel(ctx context.Context, items []map[string]any) string {
	e.cache.mu.Lock()
	l := e.cache.statusLabel
	e.cache.mu.Unlock()
	if l != "" {
		return l
	}
	tried := map[string]bool{}
	for _, it := range items {
		key := fmt.Sprintf("%d-%d", intOf(it["projectId"]), intOf(it["typeId"]))
		if tried[key] || len(tried) >= 5 {
			continue
		}
		tried[key] = true
		res, err := e.C.Get(ctx, "/work_packages/schemas/"+key, nil)
		if err != nil {
			continue
		}
		if st, ok := res["status"].(map[string]any); ok && hal.String(st["name"]) != "" {
			l = hal.String(st["name"])
			break
		}
	}
	e.cache.mu.Lock()
	e.cache.statusLabel = l
	e.cache.mu.Unlock()
	return l
}

// buildHistory turns status changes into spans, from creation to now.
func buildHistory(created time.Time, current string, changes []statusChangeAt, statuses map[string]statusInfo) wpHistory {
	h := wpHistory{Created: created}
	initial := current
	if len(changes) > 0 && changes[0].From != "" {
		initial = changes[0].From
	} else if len(changes) > 0 {
		initial = changes[0].To
		changes = changes[1:]
	}
	h.Spans = []statusSpan{{Status: initial, Entered: created}}
	for _, c := range changes {
		last := &h.Spans[len(h.Spans)-1]
		if c.To == last.Status {
			continue
		}
		last.Left = c.At
		h.Spans = append(h.Spans, statusSpan{Status: c.To, Entered: c.At})
	}
	for i := len(h.Spans) - 1; i >= 0 && statuses[h.Spans[i].Status].Closed; i-- {
		h.Closed = h.Spans[i].Entered
	}
	return h
}

type statusChangeAt struct {
	At       time.Time
	From, To string
}

// histories reads the status history of each work package (one activities
// request each, in parallel). Those whose history cannot be read are left
// out with a warning.
func (e *Env) histories(ctx context.Context, items []map[string]any) []wpHistory {
	statuses := e.statusByName(ctx)
	label := e.statusLabel(ctx, items)
	out := make([]*wpHistory, len(items))
	errs := make([]error, len(items))
	forEach(len(items), func(i int) {
		it := items[i]
		acts, _, err := e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/activities", intOf(it["id"])), nil, 0)
		if err != nil {
			errs[i] = err
			return
		}
		var changes []statusChangeAt
		for _, a := range acts {
			at, _ := time.Parse(time.RFC3339, hal.String(a["createdAt"]))
			details, _ := a["details"].([]any)
			for _, d := range details {
				dm, _ := d.(map[string]any)
				if from, to, ok := statusChange(hal.String(dm["html"]), label, statuses); ok {
					changes = append(changes, statusChangeAt{At: at, From: from, To: to})
				}
			}
		}
		created, _ := time.Parse(time.RFC3339, hal.String(it["createdAt"]))
		h := buildHistory(created, hal.String(it["status"]), changes, statuses)
		h.WP = it
		out[i] = &h
	})
	var res []wpHistory
	var failed []string
	for i, h := range out {
		if h == nil {
			failed = append(failed, fmt.Sprintf("#%d (%v)", intOf(items[i]["id"]), errs[i]))
			continue
		}
		res = append(res, *h)
	}
	if len(failed) > 0 {
		e.warnf("status history unreadable, left out: %s", strings.Join(failed, ", "))
	}
	return res
}

// closedBetween keeps the work packages whose last close falls within
// [from, to] (dates in the user's time zone, either may be empty) and sets
// their closedAt.
func (e *Env) closedBetween(ctx context.Context, items []map[string]any, from, to string) []wpHistory {
	loc := e.location(ctx)
	var out []wpHistory
	for _, h := range e.histories(ctx, items) {
		if h.Closed.IsZero() {
			continue
		}
		day := h.Closed.In(loc).Format(time.DateOnly)
		if (from != "" && day < from) || (to != "" && day > to) {
			continue
		}
		h.WP["closedAt"] = apiTime(h.Closed)
		out = append(out, h)
	}
	return out
}

// parseSince accepts 30d, 12w or anything ParseDate does.
func parseSince(s string, now time.Time) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, err := strconv.Atoi(strings.TrimRight(s, "dw")); err == nil && n >= 0 && len(s) > 1 && strings.ContainsAny(s[len(s)-1:], "dw") {
		if s[len(s)-1] == 'w' {
			n *= 7
		}
		return parseDateAt(fmt.Sprintf("-%dd", n), now)
	}
	return parseDateAt(s, now)
}

var wpFlow = &Op{
	Cmd:   []string{"wp", "flow"},
	Short: "Throughput and cycle time: work packages closed in a period, with their status history",
	Long: `Measure the work packages that were closed in the period (default the last
30 days): when each one entered and left every status, from its activity
history.

  throughput   how many were closed, in total and per week (weeks start on
               Monday, in your OpenProject time zone)
  cycle time   days from the first entry into a start status to the close:
               p50, p85 (85% took that long or less) and max
  lead time    days from creation to the close
  per status   where the cycle time went: days summed over the work packages,
               share of the total and p50/p85 per work package

A work package counts when its current status is closed (isClosed) and it
entered the final run of closed statuses within the period; closed means the
status' own flag, so "Rejected" counts if it is a closed status. The start
statuses default to the "doing" ones (neither the default status nor closed);
--start-status picks others. Work packages closed without passing through a
start status count in the throughput only. Days are calendar days.

It reads one activity history per closed work package, so long periods on
busy projects take a while.`,
	Example: `  opcli wp flow -p demo-project
  opcli wp flow -p demo-project --since 12w --type Task
  opcli wp flow --project all --since 2026-01-01 --until 2026-03-31 -o json
  opcli wp flow -p demo-project --start-status "In progress" --list`,
	Params: []Param{
		pProject("Project id, identifier or name; 'all' for every project (default: configured project)"),
		{Name: "since", Kind: String, Default: "30d", Desc: "Start of the period: 30d, 12w, YYYY-MM-DD, week, month"},
		{Name: "until", Kind: String, Desc: "End of the period, inclusive (YYYY-MM-DD, today, -7d; default today)"},
		{Name: "type", Kind: String, Short: "t", Desc: "Comma-separated type names/ids"},
		{Name: "assignee", Kind: String, Short: "a", Desc: "me, none, or login/email/name/id"},
		{Name: "start_status", Kind: String, Desc: "Comma-separated statuses where the cycle starts (default: every status neither default nor closed)"},
		{Name: "list", Kind: Bool, Desc: "Also list the closed work packages in text output (JSON always has them)"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		now := time.Now().In(e.location(ctx))
		from, err := parseSince(a.String("since"), now)
		if err != nil {
			return nil, err
		}
		to, err := parseDateAt(a.String("until"), now)
		if err != nil {
			return nil, err
		}
		if to == "" {
			to = now.Format(time.DateOnly)
		}
		if from > to {
			return nil, fmt.Errorf("--since %s is after --until %s", from, to)
		}

		f := NewFilters().Add("status", "c").Add("updatedAt", "<>d", from, "")
		project := a.String("project")
		projectName := "all"
		if !strings.EqualFold(project, "all") && (project != "" || e.DefaultProject != "") {
			pid, err := e.ProjectID(ctx, project)
			if err != nil {
				return nil, err
			}
			f = f.Add("project", "=", pid)
			projectName = firstNonEmpty(project, e.DefaultProject)
		}
		if a.Has("type") {
			ids, err := e.resolveMany(ctx, a.Strings("type"), e.TypeID)
			if err != nil {
				return nil, err
			}
			f = f.Add("type", "=", ids...)
		}
		if a.Has("assignee") {
			if f, err = e.userFilter(ctx, f, "assignee", a.String("assignee")); err != nil {
				return nil, err
			}
		}

		statuses := e.statusInfos(ctx)
		if len(statuses) == 0 {
			return nil, fmt.Errorf("cannot read the statuses, needed to tell open from closed")
		}
		start := map[string]bool{}
		if a.Has("start_status") {
			for _, ref := range a.Strings("start_status") {
				id, err := e.StatusID(ctx, ref)
				if err != nil {
					return nil, err
				}
				start[statuses[id].Name] = true
			}
		} else {
			for _, s := range statuses {
				if s.category() == "doing" {
					start[s.Name] = true
				}
			}
		}

		q := url.Values{"filters": {f.JSON()}, "sortBy": {stableSort}}
		items, _, err := e.C.CollectWith(ctx, "/work_packages", q, 0, e.wpPageFallback("/work_packages"))
		if err != nil {
			return nil, err
		}
		wps := hal.FlattenAll(items)
		for _, it := range wps {
			e.decorateWP(ctx, it)
		}
		hs := e.closedBetween(ctx, wps, from, to)
		return &flowResult{Report: flowReport(hs, start, statuses, projectName, from, to, e.location(ctx)), List: a.Bool("list")}, nil
	},
	Text: renderFlow,
}

// flowResult is the flow report plus the --list switch, which only the
// text renderer reads.
type flowResult struct {
	Report map[string]any
	List   bool
}

func (f *flowResult) MarshalJSON() ([]byte, error) { return json.Marshal(f.Report) }

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// flowReport aggregates the histories of the work packages closed in the
// period. Kept free of I/O so it can be tested with synthetic histories.
func flowReport(hs []wpHistory, start map[string]bool, statuses map[int]statusInfo, project, from, to string, loc *time.Location) map[string]any {
	position := map[string]int{}
	for _, s := range statuses {
		position[s.Name] = s.Position
	}
	startNames := make([]string, 0, len(start))
	for n := range start {
		startNames = append(startNames, n)
	}
	sort.Slice(startNames, func(i, j int) bool { return position[startNames[i]] < position[startNames[j]] })

	sort.Slice(hs, func(i, j int) bool { return hs[i].Closed.Before(hs[j].Closed) })
	var cycle, lead []float64
	perStatus := map[string][]float64{}
	weekly := map[string]int{}
	wps := make([]map[string]any, 0, len(hs))
	for _, h := range hs {
		lead = append(lead, days(h.Closed.Sub(h.Created)))
		weekly[monday(h.Closed.In(loc)).Format(time.DateOnly)]++

		var started time.Time
		for _, s := range h.Spans {
			if start[s.Status] && s.Entered.Before(h.Closed) {
				started = s.Entered
				break
			}
		}
		wp := map[string]any{
			"id": h.WP["id"], "subject": h.WP["subject"], "type": h.WP["type"],
			"project": h.WP["project"], "status": h.WP["status"], "assignee": h.WP["assignee"],
			"created": apiTime(h.Created), "closed": apiTime(h.Closed),
			"leadDays": days(h.Closed.Sub(h.Created)), "url": h.WP["url"],
		}
		if !started.IsZero() {
			cd := days(h.Closed.Sub(started))
			cycle = append(cycle, cd)
			wp["started"] = apiTime(started)
			wp["cycleDays"] = cd
			mine := map[string]float64{}
			for _, s := range h.Spans {
				if s.Entered.Before(started) || !s.Entered.Before(h.Closed) {
					continue
				}
				end := h.Closed
				if !s.Left.IsZero() && s.Left.Before(end) {
					end = s.Left
				}
				mine[s.Status] += end.Sub(s.Entered).Hours() / 24
			}
			for n, d := range mine {
				perStatus[n] = append(perStatus[n], d)
			}
		}
		history := make([]map[string]any, len(h.Spans))
		for i, s := range h.Spans {
			entry := map[string]any{"status": s.Status, "entered": apiTime(s.Entered)}
			if !s.Left.IsZero() {
				entry["left"] = apiTime(s.Left)
				entry["days"] = days(s.Left.Sub(s.Entered))
			}
			history[i] = entry
		}
		wp["history"] = history
		wps = append(wps, wp)
	}

	total := 0.0
	for _, ds := range perStatus {
		for _, d := range ds {
			total += d
		}
	}
	names := make([]string, 0, len(perStatus))
	for n := range perStatus {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return position[names[i]] < position[names[j]] })
	byStatus := make([]map[string]any, 0, len(names))
	for _, n := range names {
		sum := 0.0
		for _, d := range perStatus[n] {
			sum += d
		}
		share := 0.0
		if total > 0 {
			share = math.Round(sum/total*1000) / 10
		}
		byStatus = append(byStatus, map[string]any{"name": n, "days": round1(sum), "share": share, "perWP": stats(perStatus[n])})
	}

	var weeks []map[string]any
	fromDay, _ := time.ParseInLocation(time.DateOnly, from, loc)
	toDay, _ := time.ParseInLocation(time.DateOnly, to, loc)
	for w := monday(fromDay); !w.After(toDay); w = w.AddDate(0, 0, 7) {
		entry := map[string]any{"week": w.Format(time.DateOnly), "count": weekly[w.Format(time.DateOnly)]}
		if w.Before(fromDay) || w.AddDate(0, 0, 6).After(toDay) {
			entry["partial"] = true
		}
		weeks = append(weeks, entry)
	}

	return map[string]any{
		"project": project, "from": from, "to": to, "startStatuses": startNames,
		"throughput": len(hs), "weekly": weeks,
		"cycleTime": stats(cycle), "leadTime": stats(lead),
		"statuses": byStatus, "workPackages": wps,
	}
}

func monday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func days(d time.Duration) float64 { return round1(d.Hours() / 24) }

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// stats is count, p50, p85 (nearest rank) and max.
func stats(xs []float64) map[string]any {
	out := map[string]any{"count": len(xs)}
	if len(xs) == 0 {
		return out
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	rank := func(p float64) float64 { return round1(s[int(math.Ceil(p*float64(len(s))))-1]) }
	out["p50"], out["p85"], out["max"] = rank(0.5), rank(0.85), round1(s[len(s)-1])
	return out
}

func renderFlow(w io.Writer, v any) error {
	fr, ok := v.(*flowResult)
	if !ok {
		return RenderJSON(w, v)
	}
	r := fr.Report
	ct, _ := r["cycleTime"].(map[string]any)
	lt, _ := r["leadTime"].(map[string]any)
	fmt.Fprintf(w, "Project %s, %s to %s; cycle starts at: %s\n", r["project"], r["from"], r["to"], strings.Join(r["startStatuses"].([]string), ", "))
	fmt.Fprintf(w, "Throughput: %d closed\n", r["throughput"])
	fmt.Fprintf(w, "Cycle time (days): %s\n", statsText(ct))
	fmt.Fprintf(w, "Lead time (days):  %s\n", statsText(lt))
	fmt.Fprintln(w, "\nWEEK        CLOSED")
	for _, wk := range r["weekly"].([]map[string]any) {
		mark := ""
		if wk["partial"] == true {
			mark = " (partial)"
		}
		fmt.Fprintf(w, "%s  %d%s\n", wk["week"], wk["count"], mark)
	}
	if st := r["statuses"].([]map[string]any); len(st) > 0 {
		fmt.Fprintf(w, "\n%-24s %8s %6s  %s\n", "STATUS", "DAYS", "SHARE", "PER WP")
		for _, s := range st {
			fmt.Fprintf(w, "%-24s %8s %5s%%  %s\n", s["name"], hal.String(s["days"]), hal.String(s["share"]), statsText(s["perWP"].(map[string]any)))
		}
	}
	if wps, _ := r["workPackages"].([]map[string]any); fr.List && len(wps) > 0 {
		fmt.Fprintf(w, "\n%-8s %-12s %-14s %8s  %s\n", "ID", "CLOSED", "TYPE", "CYCLE", "SUBJECT")
		for _, wp := range wps {
			fmt.Fprintf(w, "%-8s %-12s %-14s %8s  %s\n", hal.String(wp["id"]), hal.String(wp["closed"])[:10], hal.String(wp["type"]), hal.String(wp["cycleDays"]), hal.String(wp["subject"]))
		}
	}
	return nil
}

func statsText(s map[string]any) string {
	if intOf(s["count"]) == 0 {
		return "n=0"
	}
	return fmt.Sprintf("n=%d p50=%s p85=%s max=%s", intOf(s["count"]), hal.String(s["p50"]), hal.String(s["p85"]), hal.String(s["max"]))
}
