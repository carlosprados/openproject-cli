package ops

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFieldChange(t *testing.T) {
	cases := []struct{ html, raw, field, from, to, text string }{
		{`<strong>Estado</strong> cambiado de <i>Nuevo</i> a <i>En curso</i>`, "", "Estado", "Nuevo", "En curso", ""},
		{`<strong>Assignee</strong> set to <i>Ana &amp; Co</i>`, "", "Assignee", "", "Ana & Co", ""},
		{`<strong>Version</strong> deleted (<strike><i>v1.2</i></strike>)`, "", "Version", "v1.2", "", ""},
		{`<strong>Scheduling mode</strong> set to Manual`, "Scheduling mode set to Manual", "Scheduling mode", "", "", "Scheduling mode set to Manual"},
		{`no markup`, "Something odd", "", "", "", "Something odd"},
	}
	for _, c := range cases {
		f, from, to, text := fieldChange(c.html, c.raw)
		if f != c.field || from != c.from || to != c.to || text != c.text {
			t.Errorf("%s: got %q %q %q %q", c.html, f, from, to, text)
		}
	}
}

func TestActivityEvents(t *testing.T) {
	wp := map[string]any{"id": 42, "displayId": "DEMO-7", "subject": "Login", "project": "Demo"}
	user := map[string]any{"_links": map[string]any{"user": map[string]any{"href": "/api/v3/users/5"}}}
	act := func(id, version int, at, comment string, details ...string) map[string]any {
		a := map[string]any{"id": id, "version": version, "createdAt": at, "comment": map[string]any{"raw": comment}, "_links": user["_links"]}
		var ds []any
		for _, d := range details {
			ds = append(ds, map[string]any{"html": d, "raw": d})
		}
		a["details"] = ds
		return a
	}
	acts := []map[string]any{
		act(1, 1, "2026-11-10T09:00:00Z", "", `<strong>Type</strong> set to <i>Task</i>`),
		act(2, 2, "2026-11-01T09:00:00Z", "too early"),
		act(3, 3, "2026-11-12T09:00:00Z", "Done", `<strong>Status</strong> changed from <i>New</i> to <i>Closed</i>`,
			`<strong>Description</strong> set (<a class="diff-details" href="/journals/3/diff/description?x=1&amp;y=2">Details</a>)`),
		act(4, 4, "2026-11-20T09:00:00Z", "too late"),
	}
	start := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	web := func(p string) string { return "https://op.example" + p }

	evs := activityEvents(wp, acts, start, end, nil, web)
	var got []string
	for _, ev := range evs {
		got = append(got, ev.Kind+":"+ev.Field+":"+ev.From+">"+ev.To+":"+ev.Comment+":"+ev.Diff)
	}
	want := "created::>:: comment::>:Done: field:Status:New>Closed:: field:Description:>::https://op.example/journals/3/diff/description?x=1&y=2"
	if g := strings.Join(got, " "); g != want {
		t.Fatalf("events:\n%s\nwant:\n%s", g, want)
	}
	if evs[0].DisplayID != "DEMO-7" || evs[0].userID != 5 || evs[0].At != "2026-11-10T09:00:00Z" {
		t.Fatalf("base fields: %+v", evs[0])
	}

	only := activityEvents(wp, acts, start, end, map[string]bool{"comment": true}, web)
	if len(only) != 1 || only[0].Kind != "comment" {
		t.Fatalf("--kind comment: %+v", only)
	}

	var buf bytes.Buffer
	r := &changesResult{Project: "Demo", Since: "2026-11-10", Until: "2026-11-17", Count: 1, WorkPackages: 1,
		Events: []changeEvent{{At: "2026-11-12T09:00:00Z", WP: 42, DisplayID: "DEMO-7", User: "Ana", Kind: "field", Field: "Status", From: "New", To: "Closed"}}}
	if err := renderChanges(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "#42 DEMO-7") || !strings.Contains(buf.String(), "Status: New → Closed") {
		t.Fatalf("text:\n%s", buf.String())
	}
}
