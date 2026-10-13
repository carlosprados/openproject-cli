package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Meetings: the meeting itself (time, place, participants, state), its
// agenda items (free text or a work package) and the outcomes recorded on
// each item while the meeting is in progress.

func init() {
	registerGroup(&Group{
		Name:    "meeting",
		Aliases: []string{"meetings"},
		Short:   "Meetings: schedule, agenda, participants and outcomes",
		Long: `A meeting has a start, a duration, a location (a room or the video call
link), participants and an agenda. Agenda items are free text or a work
package; outcomes (decisions, follow-up work packages) are recorded on an
item while the meeting is in progress.

Times accept "YYYY-MM-DD HH:MM", "tomorrow 9:30", "16:00" (today) or "+2h",
in your OpenProject time zone. Durations accept 45m, 1h30m, 1.5.`,
	})
	registerGroup(&Group{Name: "meeting agenda", Aliases: []string{"agenda"}, Short: "Agenda items of a meeting"})
	registerGroup(&Group{Name: "meeting outcome", Aliases: []string{"outcomes"}, Short: "Outcomes recorded on agenda items"})
	register(meetingList, meetingGet, meetingCreate, meetingUpdate, meetingDelete,
		agendaAdd, agendaUpdate, agendaDelete, outcomeAdd, outcomeDelete)
}

var meetingStates = []string{"draft", "open", "in_progress", "closed", "cancelled"}

var meetingList = &Op{
	Cmd:     []string{"meeting", "list"},
	Aliases: []string{"ls"},
	Short:   "List meetings",
	Example: `  opcli meeting list --when upcoming
  opcli meeting list --when upcoming --participant me
  opcli meeting list -p demo-project --when past --max 10`,
	Params: []Param{
		pProject("Only meetings of this project (default: all visible)"),
		{Name: "when", Kind: String, Enum: []string{"upcoming", "past"}, Desc: "upcoming (not yet ended, soonest first) or past (started before now, latest first)"},
		{Name: "participant", Kind: String, Desc: "Only meetings this user is invited to: me, login, email, name or id"},
		pMax(50), pPage,
	},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "TITLE", Key: "title"}, {Header: "START", Key: "startTime"}, {Header: "DURATION", Key: "duration", Fmt: hoursFmt}, {Header: "LOCATION", Key: "location"}, {Header: "PROJECT", Key: "project"}, {Header: "STATE", Key: "state"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		f := NewFilters()
		if a.Has("project") {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			f = f.Add("project_id", "=", pid)
		}
		if a.Has("participant") {
			uid, err := e.UserID(ctx, a.String("participant"))
			if err != nil {
				return nil, err
			}
			f = f.Add("invited_user_id", "=", uid)
		}
		order := "desc"
		if w := a.String("when"); w != "" {
			f = append(f, map[string]any{"time": map[string]any{"operator": w, "values": []any{}}})
			if w == "upcoming" {
				order = "asc"
			}
		}
		q := url.Values{"filters": {f.JSON()}, "sortBy": {`[["start_time","` + order + `"]]`}}
		l, err := e.list(ctx, "/meetings", q, a)
		if err != nil {
			return nil, err
		}
		for _, it := range l.Items {
			delete(it, "_type")
		}
		return l, nil
	},
}

var meetingGet = &Op{
	Cmd:     []string{"meeting", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a meeting with its participants, agenda items and outcomes",
	Example: `  opcli meeting get 3`,
	Params:  []Param{pID("Meeting")},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := url.PathEscape(a.String("id"))
		res, err := e.C.Get(ctx, "/meetings/"+id, nil)
		if err != nil {
			return nil, err
		}
		m := hal.Flatten(res)
		delete(m, "_type")
		items, _, err := e.C.Collect(ctx, "/meetings/"+id+"/agenda_items", nil, 0)
		if err != nil {
			return nil, err
		}
		agenda := make([]map[string]any, len(items))
		for i, raw := range items {
			it := trimAgendaItem(hal.Flatten(raw, "outcomes"))
			delete(it, "meeting")
			delete(it, "meetingId")
			oc, _ := it["outcomes"].([]map[string]any)
			for _, o := range oc {
				for _, k := range []string{"_type", "agendaItem", "agendaItemId"} {
					delete(o, k)
				}
			}
			if len(oc) == 0 {
				delete(it, "outcomes")
			}
			agenda[i] = it
		}
		m["agendaItems"] = agenda
		return m, nil
	},
	Text: renderMeeting,
}

func renderMeeting(w io.Writer, v any) error {
	m := v.(map[string]any)
	rec := make(map[string]any, len(m))
	for k, x := range m {
		rec[k] = x
	}
	delete(rec, "agendaItems")
	rec["duration"] = hal.HoursString(m["duration"])
	if err := renderRecord(w, rec); err != nil {
		return err
	}
	agenda, _ := m["agendaItems"].([]map[string]any)
	fmt.Fprintf(w, "\nagenda (%d):\n", len(agenda))
	for i, it := range agenda {
		head := hal.String(it["title"])
		if wp := hal.String(it["workPackage"]); wp != "" {
			head = strings.TrimSpace(fmt.Sprintf("#%s %s %s", hal.String(it["workPackageId"]), wp, head))
		}
		parts := []string{fmt.Sprintf("%d. [%s] %s", i+1, hal.String(it["id"]), head)}
		if d := intOf(it["durationInMinutes"]); d > 0 {
			parts = append(parts, fmt.Sprintf("%dm", d))
		}
		if p := hal.String(it["presenter"]); p != "" {
			parts = append(parts, p)
		}
		fmt.Fprintf(w, "  %s\n", strings.Join(parts, " · "))
		if n := strings.TrimSpace(hal.String(it["notes"])); n != "" {
			fmt.Fprintf(w, "     %s\n", strings.ReplaceAll(n, "\n", "\n     "))
		}
		oc, _ := it["outcomes"].([]map[string]any)
		for _, o := range oc {
			line := strings.TrimSpace(hal.String(o["notes"]))
			if wp := hal.String(o["workPackage"]); wp != "" {
				line = strings.TrimSpace(fmt.Sprintf("#%s %s %s", hal.String(o["workPackageId"]), wp, line))
			}
			fmt.Fprintf(w, "     → [%s] %s\n", hal.String(o["id"]), line)
		}
	}
	return nil
}

var meetingParams = []Param{
	{Name: "title", Kind: String, Desc: "Title"},
	{Name: "start", Kind: String, Desc: `Start: "YYYY-MM-DD HH:MM", "tomorrow 9:30", "16:00", "+2h" (your OpenProject time zone)`},
	{Name: "duration", Kind: String, Desc: "Duration: 45m, 1h30m, 1.5 (hours)"},
	{Name: "location", Kind: String, Desc: "Room or video call link (Google Meet, Teams, ...)"},
	{Name: "notify", Kind: Bool, Desc: "Email the participants about this change"},
}

var meetingCreate = &Op{
	Cmd:     []string{"meeting", "create"},
	Aliases: []string{"new", "add"},
	Short:   "Schedule a meeting",
	Long: `Schedule a one-off meeting in a project. You are added to the participants;
--participant invites others. New meetings start as a draft; '--state open'
opens it at once. The server validates the meeting before it is created.

Add the agenda afterwards with 'opcli meeting agenda add'.`,
	Example: `  opcli meeting create --title "Sprint review" --start "2026-10-20 10:00" --duration 45m \
    --location https://meet.google.com/abc-defg-hij --participant jane.doe,john.roe
  opcli meeting create -p demo-project --title "Kick-off" --start "tomorrow 9:30" --state open --notify`,
	Params: append([]Param{
		pProject(""),
		{Name: "participant", Kind: Strings, Desc: "Invite these users (me, login, email, name or id)"},
		{Name: "state", Kind: String, Enum: meetingStates, Desc: "Initial state (default: draft)"},
	}, meetingParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if a.String("title") == "" || a.String("start") == "" {
			return nil, fmt.Errorf("--title and --start are required")
		}
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		body, err := e.meetingPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		links := body["_links"].(map[string]any)
		links["project"] = hal.Ref(href("projects", pid))
		if _, ok := body["duration"]; !ok {
			body["duration"] = "PT1H"
		}
		if users := a.Strings("participant"); len(users) > 0 {
			hrefs, err := e.userHrefs(ctx, append([]string{"me"}, users...))
			if err != nil {
				return nil, err
			}
			links["participants"] = refs(hrefs)
		}
		if err := e.validate(ctx, "/meetings/form", body); err != nil {
			return nil, err
		}
		res, err := e.C.Post(ctx, "/meetings", body)
		if err != nil {
			return nil, err
		}
		return flatMeeting(res), nil
	},
}

var meetingUpdate = &Op{
	Cmd:     []string{"meeting", "update"},
	Aliases: []string{"edit", "set"},
	Short:   "Reschedule a meeting, change its participants or move it along its states",
	Long: `Change only what you pass. --participant adds users and
--remove-participant removes them. --state moves the meeting along
draft → open → in_progress → closed (or cancelled); outcomes can only be
recorded while it is in_progress.`,
	Example: `  opcli meeting update 7 --start "2026-10-21 10:00" --notify
  opcli meeting update 7 --participant jane.doe --remove-participant john.roe
  opcli meeting update 7 --state in_progress
  opcli meeting update 7 --location https://teams.microsoft.com/l/meetup-join/...`,
	Params: append([]Param{
		pID("Meeting"),
		{Name: "participant", Kind: Strings, Desc: "Add these users (me, login, email, name or id)"},
		{Name: "remove_participant", Kind: Strings, Desc: "Remove these users"},
		{Name: "state", Kind: String, Enum: meetingStates, Desc: "New state"},
	}, meetingParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := url.PathEscape(a.String("id"))
		body, err := e.meetingPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		cur, err := e.C.Get(ctx, "/meetings/"+id, nil)
		if err != nil {
			return nil, err
		}
		if a.Has("participant") || a.Has("remove_participant") {
			add, err := e.userHrefs(ctx, a.Strings("participant"))
			if err != nil {
				return nil, err
			}
			drop, err := e.userHrefs(ctx, a.Strings("remove_participant"))
			if err != nil {
				return nil, err
			}
			var hrefs []string
			for _, l := range linkList(cur, "participants") {
				if !containsStr(drop, l) && !containsStr(hrefs, l) {
					hrefs = append(hrefs, l)
				}
			}
			for _, h := range add {
				if !containsStr(hrefs, h) {
					hrefs = append(hrefs, h)
				}
			}
			body["_links"].(map[string]any)["participants"] = refs(hrefs)
		}
		body["lockVersion"] = cur["lockVersion"]
		if err := e.validate(ctx, "/meetings/"+id+"/form", body); err != nil {
			return nil, err
		}
		res, err := e.C.Patch(ctx, "/meetings/"+id, body)
		if err != nil {
			return nil, err
		}
		return flatMeeting(res), nil
	},
}

var meetingDelete = &Op{
	Cmd:     []string{"meeting", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete a meeting with its agenda and outcomes",
	Example: `  opcli meeting delete 7 --yes`,
	Params:  []Param{pID("Meeting")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if err := e.C.Delete(ctx, "/meetings/"+url.PathEscape(a.String("id"))); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted meeting " + a.String("id")}, nil
	},
}

func (e *Env) meetingPayload(ctx context.Context, a Args) (map[string]any, error) {
	body := map[string]any{"_links": map[string]any{}}
	for _, k := range []string{"title", "location"} {
		if a.Has(k) {
			body[k] = a.String(k)
		}
	}
	if a.Has("start") {
		t, err := ParseDateTime(a.String("start"), e.location(ctx))
		if err != nil {
			return nil, err
		}
		body["startTime"] = apiTime(t)
	}
	if a.Has("duration") {
		h, err := hal.ParseHours(a.String("duration"))
		if err != nil {
			return nil, err
		}
		if h <= 0 {
			return nil, fmt.Errorf("duration must be positive")
		}
		body["duration"] = hal.ISOHours(h)
	}
	if a.Has("state") {
		body["state"] = strings.ToLower(a.String("state"))
	}
	if a.Has("notify") {
		body["notify"] = a.Bool("notify")
	}
	return body, nil
}

func flatMeeting(res map[string]any) map[string]any {
	m := hal.Flatten(res)
	delete(m, "_type")
	return m
}

func (e *Env) userHrefs(ctx context.Context, users []string) ([]string, error) {
	var out []string
	for _, u := range users {
		id, err := e.UserID(ctx, u)
		if err != nil {
			return nil, err
		}
		if h := href("users", id); !containsStr(out, h) {
			out = append(out, h)
		}
	}
	return out, nil
}

func refs(hrefs []string) []any {
	out := make([]any, len(hrefs))
	for i, h := range hrefs {
		out[i] = hal.Ref(h)
	}
	return out
}

// linkList returns the hrefs of a link array (participants, ...).
func linkList(res map[string]any, name string) []string {
	links, _ := res["_links"].(map[string]any)
	arr, _ := links[name].([]any)
	var out []string
	for _, l := range arr {
		if m, ok := l.(map[string]any); ok {
			out = append(out, hal.String(m["href"]))
		}
	}
	return out
}

var agendaParams = []Param{
	{Name: "title", Kind: String, Desc: "Title (optional for a work package item)"},
	{Name: "wp", Kind: String, Desc: "Discuss this work package (id)"},
	{Name: "duration", Kind: String, Desc: "Time slot: 10m, 1h, 0.5"},
	{Name: "notes", Kind: String, File: true, Desc: "Notes (Markdown)"},
	{Name: "presenter", Kind: String, Desc: "Who presents it: me, login, email, name or id ('none' clears)"},
}

var agendaAdd = &Op{
	Cmd:   []string{"meeting", "agenda", "add"},
	Short: "Add an agenda item (free text or a work package) to a meeting",
	Example: `  opcli meeting agenda add 7 --title "Release status" --duration 10m --presenter jane.doe
  opcli meeting agenda add 7 --wp 42 --duration 5m
  opcli meeting agenda add 7 --title "Risks" --notes-file risks.md`,
	Params: append([]Param{pID("Meeting")}, agendaParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if a.String("title") == "" && a.String("wp") == "" {
			return nil, fmt.Errorf("pass --title, --wp or both")
		}
		body, err := e.agendaPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		mid, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, fmt.Errorf("invalid meeting id %q", a.String("id"))
		}
		body["_links"].(map[string]any)["meeting"] = hal.Ref(href("meetings", mid))
		if _, ok := body["itemType"]; !ok {
			body["itemType"] = "simple"
		}
		res, err := e.C.Post(ctx, "/meeting_agenda_items", body)
		if err != nil {
			return nil, err
		}
		return flatAgendaItem(res), nil
	},
}

var agendaUpdate = &Op{
	Cmd:   []string{"meeting", "agenda", "update"},
	Short: "Change an agenda item",
	Example: `  opcli meeting agenda update 37 --duration 15m --notes "Moved to Friday"
  opcli meeting agenda update 37 --presenter none`,
	Params: append([]Param{pID("Agenda item")}, agendaParams...),
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		path := "/meeting_agenda_items/" + url.PathEscape(a.String("id"))
		body, err := e.agendaPayload(ctx, a)
		if err != nil {
			return nil, err
		}
		cur, err := e.C.Get(ctx, path, nil)
		if err != nil {
			return nil, err
		}
		body["lockVersion"] = cur["lockVersion"]
		res, err := e.C.Patch(ctx, path, body)
		if err != nil {
			return nil, err
		}
		return flatAgendaItem(res), nil
	},
}

var agendaDelete = &Op{
	Cmd:     []string{"meeting", "agenda", "delete"},
	Aliases: []string{"rm"},
	Short:   "Remove an agenda item",
	Example: `  opcli meeting agenda delete 37 --yes`,
	Params:  []Param{pID("Agenda item")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if err := e.C.Delete(ctx, "/meeting_agenda_items/"+url.PathEscape(a.String("id"))); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted agenda item " + a.String("id")}, nil
	},
}

func (e *Env) agendaPayload(ctx context.Context, a Args) (map[string]any, error) {
	links := map[string]any{}
	body := map[string]any{"_links": links}
	if a.Has("title") {
		body["title"] = a.String("title")
	}
	if a.Has("wp") {
		wp, err := WorkPackageID(a.String("wp"))
		if err != nil {
			return nil, err
		}
		links["workPackage"] = hal.Ref(href("work_packages", wp))
		body["itemType"] = "work_package"
	}
	if a.Has("duration") {
		h, err := hal.ParseHours(a.String("duration"))
		if err != nil {
			return nil, err
		}
		body["durationInMinutes"] = int(math.Round(h * 60))
	}
	if a.Has("notes") {
		body["notes"] = hal.Text(a.String("notes"))
	}
	if a.Has("presenter") {
		h, err := e.UserHref(ctx, a.String("presenter"))
		if err != nil {
			return nil, err
		}
		links["presenter"] = map[string]any{"href": nilIfEmpty(h)}
	}
	return body, nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func flatAgendaItem(res map[string]any) map[string]any { return trimAgendaItem(hal.Flatten(res)) }

func trimAgendaItem(m map[string]any) map[string]any {
	for _, k := range []string{"_type", "outcomesIds", "lockVersion"} {
		delete(m, k)
	}
	if s, _ := m["section"].(string); s == "" {
		delete(m, "section")
		delete(m, "sectionId")
	}
	return m
}

var outcomeAdd = &Op{
	Cmd:   []string{"meeting", "outcome", "add"},
	Short: "Record an outcome (decision or follow-up work package) on an agenda item",
	Long: `Record what came out of an agenda item: --notes for a decision or
information, --wp to link a follow-up work package (create it first with
'opcli wp create'). The meeting must be in progress
('opcli meeting update <id> --state in_progress').`,
	Example: `  opcli meeting outcome add 37 --notes "Release moves to Friday"
  opcli meeting outcome add 37 --wp 51 --notes "Jane prepares the migration"`,
	Params: []Param{
		pID("Agenda item"),
		{Name: "notes", Kind: String, File: true, Desc: "What was decided (Markdown)"},
		{Name: "wp", Kind: String, Desc: "Follow-up work package id"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if a.String("notes") == "" && a.String("wp") == "" {
			return nil, fmt.Errorf("pass --notes, --wp or both")
		}
		item, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, fmt.Errorf("invalid agenda item id %q", a.String("id"))
		}
		links := map[string]any{"agendaItem": hal.Ref(href("meeting_agenda_items", item))}
		body := map[string]any{"kind": "information", "_links": links}
		if a.Has("notes") {
			body["notes"] = hal.Text(a.String("notes"))
		}
		if a.Has("wp") {
			wp, err := WorkPackageID(a.String("wp"))
			if err != nil {
				return nil, err
			}
			links["workPackage"] = hal.Ref(href("work_packages", wp))
			body["kind"] = "work_package"
		}
		res, err := e.C.Post(ctx, "/meeting_outcomes", body)
		if err != nil {
			return nil, e.inProgressHint(ctx, err, item)
		}
		m := hal.Flatten(res)
		delete(m, "_type")
		return m, nil
	},
}

var outcomeDelete = &Op{
	Cmd:     []string{"meeting", "outcome", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete an outcome",
	Example: `  opcli meeting outcome delete 2 --yes`,
	Params:  []Param{pID("Outcome")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		path := "/meeting_outcomes/" + url.PathEscape(a.String("id"))
		if err := e.C.Delete(ctx, path); err != nil {
			item := 0
			if o, gerr := e.C.Get(ctx, path, nil); gerr == nil {
				item = intOf(hal.IDFromHref(hal.Link(o, "agendaItem")))
			}
			return nil, e.inProgressHint(ctx, err, item)
		}
		return &Message{OK: true, Message: "Deleted outcome " + a.String("id")}, nil
	},
}

// inProgressHint explains the server's "not editable anymore": outcomes
// change only while their meeting is in progress.
func (e *Env) inProgressHint(ctx context.Context, err error, agendaItem int) error {
	var ae *client.APIError
	if !errors.As(err, &ae) || !strings.Contains(strings.Join(append(ae.Details, ae.Message), " "), "not editable") {
		return err
	}
	meeting := "<meeting-id>"
	if it, gerr := e.C.Get(ctx, fmt.Sprintf("/meeting_agenda_items/%d", agendaItem), nil); agendaItem > 0 && gerr == nil {
		meeting = fmt.Sprint(hal.IDFromHref(hal.Link(it, "meeting")))
	}
	return fmt.Errorf("%w (outcomes can only change while the meeting is in progress: opcli meeting update %s --state in_progress)", err, meeting)
}
