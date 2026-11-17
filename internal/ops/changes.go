package ops

import (
	"context"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() { register(wpChanges) }

var changeKinds = []string{"created", "comment", "field"}

// changeEvent is one thing that happened to a work package: its creation, a
// comment or one field change. An activity that both comments and changes
// fields gives one event each.
type changeEvent struct {
	At        string `json:"at"`
	WP        int    `json:"wp"`
	DisplayID string `json:"displayId,omitempty"`
	Subject   string `json:"subject"`
	Project   string `json:"project"`
	User      string `json:"user"`
	Kind      string `json:"kind"`
	Field     string `json:"field,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Text      string `json:"text,omitempty"`
	Diff      string `json:"diff,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Internal  bool   `json:"internal,omitempty"`
	Activity  int    `json:"activity"`

	userID int
	detail int
}

type changesResult struct {
	Project      string        `json:"project"`
	Since        string        `json:"since"`
	Until        string        `json:"until"`
	WorkPackages int           `json:"workPackages"`
	Count        int           `json:"count"`
	Events       []changeEvent `json:"events"`
}

var wpChanges = &Op{
	Cmd:   []string{"wp", "changes"},
	Short: "What changed in a period: creations, comments and field changes, oldest first",
	Long: `List what happened to work packages in a period (default the last 7 days),
read from their activity history: one event per creation, comment and field
change, sorted by time.

The work packages are those the wp list filters select (default: any status)
that were updated since --since, or the ids given. Each one costs one request
for its history, so long periods on busy projects take a while.

A field change carries the field's label as OpenProject shows it (in your
language; custom fields by name) and its values as text: from (absent when it
was empty) and to (absent when it was cleared). Changes whose values
OpenProject does not show (the description: only a link to the diff) have
neither, and diff, the web page with the change, when there is one;
changes it words differently keep their wording in text.`,
	Example: `  opcli wp changes -p demo-project
  opcli wp changes -p programme --subprojects --since 7d -o json
  opcli wp changes -p demo-project --since 2026-11-01 --until 2026-11-15 --kind comment,field
  opcli wp changes 42 43 --since 30d
  opcli wp changes -p demo-project -o json | jq '[.events[] | select(.user != "Jane Doe")]'`,
	Params: changesParams(),
	Text:   renderChanges,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		loc := e.location(ctx)
		now := time.Now().In(loc)
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
		kinds := map[string]bool{}
		for _, k := range splitList(a.String("kind")) {
			if !contains(changeKinds, k) {
				return nil, fmt.Errorf("--kind %q: use %s", k, strings.Join(changeKinds, ", "))
			}
			kinds[k] = true
		}
		start, _ := time.ParseInLocation(time.DateOnly, from, loc)
		end, _ := time.ParseInLocation(time.DateOnly, to, loc)
		end = end.AddDate(0, 0, 1)

		la, err := changesListArgs(ctx, e, a, from)
		if err != nil {
			return nil, err
		}
		l, err := e.listWorkPackages(ctx, la, false)
		if err != nil {
			return nil, err
		}
		perWP := make([][]changeEvent, len(l.Items))
		errs := make([]error, len(l.Items))
		forEach(len(l.Items), func(i int) {
			wp := l.Items[i]
			acts, _, err := e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/activities", intOf(wp["id"])), nil, 0)
			if err != nil {
				errs[i] = err
				return
			}
			perWP[i] = activityEvents(wp, acts, start, end, kinds, e.C.WebURL)
		})
		var failed []string
		res := &changesResult{Project: firstNonEmpty(a.String("project"), e.DefaultProject, "all"), Since: from, Until: to, Events: []changeEvent{}}
		for i, evs := range perWP {
			if errs[i] != nil {
				failed = append(failed, fmt.Sprintf("#%d (%v)", intOf(l.Items[i]["id"]), errs[i]))
				continue
			}
			if len(evs) > 0 {
				res.WorkPackages++
			}
			res.Events = append(res.Events, evs...)
		}
		if len(failed) > 0 {
			e.warnf("history unreadable, left out: %s", strings.Join(failed, ", "))
		}
		if a.String("ids") == "" && a.Bool("subprojects") {
			res.Project += " (with subprojects)"
		}
		names := map[int]string{}
		for i := range res.Events {
			ev := &res.Events[i]
			act := map[string]any{"userId": ev.userID}
			e.nameUser(ctx, act, names)
			ev.User = fmt.Sprint(act["user"])
		}
		sort.SliceStable(res.Events, func(i, j int) bool {
			x, y := res.Events[i], res.Events[j]
			if x.At != y.At {
				return x.At < y.At
			}
			if x.WP != y.WP {
				return x.WP < y.WP
			}
			if x.Activity != y.Activity {
				return x.Activity < y.Activity
			}
			return x.detail < y.detail
		})
		res.Count = len(res.Events)
		return res, nil
	},
}

func changesParams() []Param {
	ps := []Param{{Name: "ids", Kind: String, Positional: true, Rest: true, Desc: "Work package ids (instead of, or on top of, the filters)"}}
	for _, p := range wpList.Params {
		switch p.Name {
		case "page", "sort", "query_id", "updated_after", "max":
			continue
		case "status":
			p.Default = nil
			p.Desc = "open, closed, all, or comma-separated status names/ids (default: all)"
		}
		ps = append(ps, p)
	}
	return append(ps,
		Param{Name: "since", Kind: String, Default: "7d", Desc: "Start of the period: 7d, 4w, YYYY-MM-DD, week, month"},
		Param{Name: "until", Kind: String, Desc: "End of the period, inclusive (YYYY-MM-DD, today, -1d; default today)"},
		Param{Name: "kind", Kind: String, Desc: "Only these kinds, comma-separated: " + strings.Join(changeKinds, ", ")},
	)
}

// changesListArgs turns the changes args into wp list args: every status by
// default and only work packages updated since the start of the period.
func changesListArgs(ctx context.Context, e *Env, a Args, from string) (Args, error) {
	b := Args{}
	for k, v := range a {
		switch k {
		case "since", "until", "kind":
		default:
			b[k] = v
		}
	}
	if !b.Has("status") {
		b["status"] = "all"
	}
	la, err := exportListArgs(ctx, e, b)
	if err != nil {
		return nil, err
	}
	la["updated_after"] = from
	return la, nil
}

var (
	reDetailStrike = regexp.MustCompile(`<strike>\s*<i>(.*?)</i>\s*</strike>`)
	reDetailLink   = regexp.MustCompile(`<a [^>]*href="([^"]+)"`)
)

// activityEvents turns the activities of one work package that fall in
// [start, end) into events. The first activity (version 1) is the creation:
// its details only repeat the initial values, so it gives one created event.
func activityEvents(wp map[string]any, acts []map[string]any, start, end time.Time, kinds map[string]bool, webURL func(string) string) []changeEvent {
	want := func(k string) bool { return len(kinds) == 0 || kinds[k] }
	var out []changeEvent
	for _, act := range acts {
		at, err := time.Parse(time.RFC3339, hal.String(act["createdAt"]))
		if err != nil || at.Before(start) || !at.Before(end) {
			continue
		}
		f := hal.Flatten(act)
		base := changeEvent{
			At: at.UTC().Format(time.RFC3339), WP: intOf(wp["id"]), DisplayID: hal.String(wp["displayId"]),
			Subject: hal.String(wp["subject"]), Project: hal.String(wp["project"]),
			Activity: intOf(act["id"]), userID: intOf(f["userId"]), Internal: act["internal"] == true,
		}
		if base.DisplayID == fmt.Sprint(base.WP) {
			base.DisplayID = ""
		}
		if intOf(act["version"]) == 1 {
			if want("created") {
				ev := base
				ev.Kind = "created"
				out = append(out, ev)
			}
			continue
		}
		if c, _ := act["comment"].(map[string]any); c != nil && strings.TrimSpace(hal.String(c["raw"])) != "" && want("comment") {
			ev := base
			ev.Kind, ev.Comment = "comment", hal.String(c["raw"])
			out = append(out, ev)
		}
		if !want("field") {
			continue
		}
		details, _ := act["details"].([]any)
		for i, d := range details {
			dm, _ := d.(map[string]any)
			ev := base
			ev.Kind, ev.detail = "field", i
			ev.Field, ev.From, ev.To, ev.Text = fieldChange(hal.String(dm["html"]), hal.String(dm["raw"]))
			if l := reDetailLink.FindStringSubmatch(hal.String(dm["html"])); l != nil && ev.From == "" && ev.To == "" {
				ev.Text, ev.Diff = "", webURL(html.UnescapeString(l[1]))
			}
			out = append(out, ev)
		}
	}
	return out
}

// fieldChange reads one activity detail. OpenProject words it in the user's
// language, but marks the field label with <strong>, values with <i> and a
// removed value with <strike>: "changed from <i>A</i> to <i>B</i>" (two
// values), "set to <i>B</i>" (one), "deleted (<strike><i>A</i></strike>)".
// Anything else keeps its plain wording in text.
func fieldChange(detailHTML, raw string) (field, from, to, text string) {
	m := reDetailLabel.FindStringSubmatch(detailHTML)
	if m == nil {
		return "", "", "", raw
	}
	field = html.UnescapeString(m[1])
	if s := reDetailStrike.FindStringSubmatch(detailHTML); s != nil {
		return field, html.UnescapeString(s[1]), "", ""
	}
	var vals []string
	for _, v := range reDetailValue.FindAllStringSubmatch(detailHTML, -1) {
		vals = append(vals, html.UnescapeString(v[1]))
	}
	switch len(vals) {
	case 1:
		return field, "", vals[0], ""
	case 2:
		return field, vals[0], vals[1], ""
	}
	return field, "", "", raw
}

func renderChanges(w io.Writer, v any) error {
	r, ok := v.(*changesResult)
	if !ok {
		return RenderJSON(w, v)
	}
	fmt.Fprintf(w, "%s, %s → %s: %d events in %d work packages\n", r.Project, r.Since, r.Until, r.Count, r.WorkPackages)
	for _, ev := range r.Events {
		at := ev.At
		if t, err := time.Parse(time.RFC3339, ev.At); err == nil {
			at = t.Local().Format("2006-01-02 15:04")
		}
		ref := "#" + fmt.Sprint(ev.WP)
		if ev.DisplayID != "" {
			ref += " " + ev.DisplayID
		}
		var what string
		switch ev.Kind {
		case "created":
			what = "created: " + ev.Subject
		case "comment":
			what = "comment: " + oneLineText(ev.Comment, 100)
		default:
			switch {
			case ev.Text != "":
				what = ev.Text
			case ev.From != "" && ev.To != "":
				what = fmt.Sprintf("%s: %s → %s", ev.Field, ev.From, ev.To)
			case ev.To != "":
				what = fmt.Sprintf("%s: → %s", ev.Field, ev.To)
			case ev.From != "":
				what = fmt.Sprintf("%s: %s → (none)", ev.Field, ev.From)
			default:
				what = ev.Field + " changed"
			}
		}
		fmt.Fprintf(w, "%s  %-18s %-20s %s\n", at, ref, ev.User, what)
	}
	return nil
}

func oneLineText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
