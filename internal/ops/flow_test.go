package ops

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
)

// brokenSchemaOP reproduces a type whose form configuration is broken:
// every collection page holding a WP of type 9 answers 500, while the
// select path and single GETs work, except WP 3, which fails on its own.
type brokenSchemaOP struct {
	mu   sync.Mutex
	reqs []string
}

func (f *brokenSchemaOP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r.URL.Path+"?"+r.URL.RawQuery)
	f.mu.Unlock()
	types := map[int]int{1: 1, 2: 9, 3: 9, 4: 1}
	wp := func(id int) string {
		return fmt.Sprintf(`{"_type":"WorkPackage","id":%d,"subject":"S%d","customField1":"x","_links":{"project":{"href":"/api/v3/projects/4","title":"P"},"type":{"href":"/api/v3/types/%d","title":"T"},"status":{"href":"/api/v3/statuses/1","title":"New"}}}`, id, id, types[id])
	}
	q := r.URL.Query()
	switch {
	case r.URL.Path == "/api/v3/statuses":
		fmt.Fprint(w, `{"total":2,"_embedded":{"elements":[{"id":1,"name":"New","isDefault":true},{"id":12,"name":"Closed","isClosed":true}]}}`)
	case r.URL.Path == "/api/v3/work_packages" && q.Get("select") != "":
		var els []string
		for id := 1; id <= 4; id++ {
			els = append(els, fmt.Sprintf(`{"id":%d,"_links":{"project":{"href":"/api/v3/projects/4"},"type":{"href":"/api/v3/types/%d"}}}`, id, types[id]))
		}
		fmt.Fprintf(w, `{"total":4,"_embedded":{"elements":[%s]}}`, strings.Join(els, ","))
	case r.URL.Path == "/api/v3/work_packages":
		filters := q.Get("filters")
		if !strings.Contains(filters, `"id"`) || strings.Contains(filters, `"2"`) {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"_type":"Error","message":"undefined method 'filters' for nil"}`)
			return
		}
		fmt.Fprintf(w, `{"total":2,"_embedded":{"elements":[%s,%s]}}`, wp(1), wp(4))
	case r.URL.Path == "/api/v3/work_packages/2":
		fmt.Fprint(w, wp(2))
	case r.URL.Path == "/api/v3/work_packages/schemas/4-1":
		fmt.Fprint(w, `{"customField1":{"name":"Customer","type":"String"}}`)
	default:
		w.WriteHeader(500)
		fmt.Fprint(w, `{"_type":"Error","message":"boom"}`)
	}
}

func TestWPListRecoversPageWithBrokenSchema(t *testing.T) {
	srv := httptest.NewServer(&brokenSchemaOP{})
	defer srv.Close()
	var warn bytes.Buffer
	env := NewEnv(client.New(srv.URL, "k"), "")
	env.Warn = &warn

	res, err := ByCmd("wp list").Execute(context.Background(), env, Args{"project": "all", "status": "all", "max": 0})
	if err != nil {
		t.Fatal(err)
	}
	l := res.(*List)
	var ids []int
	for _, it := range l.Items {
		ids = append(ids, intOf(it["id"]))
	}
	if fmt.Sprint(ids) != "[1 2 4]" || l.Total != 4 {
		t.Errorf("ids %v total %d, want [1 2 4] of 4 (page order kept, #3 skipped)", ids, l.Total)
	}
	if l.Items[0]["statusIsClosed"] != false || l.Items[0]["statusCategory"] != "todo" {
		t.Errorf("status decoration missing: %v", l.Items[0])
	}
	w := warn.String()
	for _, want := range []string{"3 of 4 recovered", "4-9", "skipped #3", "schema of project 4, type 9 unavailable"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning lacks %q:\n%s", want, w)
		}
	}
}

func TestStatusChangeIsReadInAnyLanguage(t *testing.T) {
	known := map[string]statusInfo{"New": {}, "In progress": {}, "Closed": {Closed: true}, "R&D": {}}
	for _, c := range []struct {
		html, label, from, to string
		ok                    bool
	}{
		{`<strong>Status</strong> changed from <i>New</i> to <i>In progress</i>`, "Status", "New", "In progress", true},
		{`<strong>Estado</strong> se cambió de <i>In progress</i> a <i>Closed</i>`, "Estado", "In progress", "Closed", true},
		{`<strong>Status</strong> set to <i>R&amp;D</i>`, "Status", "", "R&D", true},
		{`<strong>Priority</strong> changed from <i>New</i> to <i>Closed</i>`, "Status", "", "", false},
		{`<strong>Status</strong> changed from <i>New</i> to <i>Unknown</i>`, "Status", "", "", false},
		{`<strong>Estado</strong> se cambió de <i>New</i> a <i>Closed</i>`, "", "New", "Closed", true},
	} {
		from, to, ok := statusChange(c.html, c.label, known)
		if ok != c.ok || from != c.from || to != c.to {
			t.Errorf("statusChange(%q, %q) = %q, %q, %v", c.html, c.label, from, to, ok)
		}
	}
}

func day(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s+"T00:00:00Z")
	return t
}

func TestFlowReport(t *testing.T) {
	statuses := map[int]statusInfo{
		1: {Name: "New", Default: true, Position: 1}, 7: {Name: "In progress", Position: 7},
		9: {Name: "In testing", Position: 9}, 12: {Name: "Closed", Closed: true, Position: 12},
		14: {Name: "Rejected", Closed: true, Position: 14},
	}
	byName := map[string]statusInfo{}
	for _, s := range statuses {
		byName[s.Name] = s
	}
	// WP 1: New 2d, In progress 3d, In testing 1d, Closed then Rejected.
	h1 := buildHistory(day("2026-09-01"), "Rejected", []statusChangeAt{
		{day("2026-09-03"), "New", "In progress"},
		{day("2026-09-06"), "In progress", "In testing"},
		{day("2026-09-07"), "In testing", "Closed"},
		{day("2026-09-09"), "Closed", "Rejected"},
	}, byName)
	h1.WP = map[string]any{"id": 1}
	if !h1.Closed.Equal(day("2026-09-07")) {
		t.Fatalf("closed at %v, want the entry into the final run of closed statuses", h1.Closed)
	}
	// WP 2: straight from New to Closed: throughput only.
	h2 := buildHistory(day("2026-09-10"), "Closed", []statusChangeAt{{day("2026-09-14"), "New", "Closed"}}, byName)
	h2.WP = map[string]any{"id": 2}
	// WP 3: reopened after a close, so not closed now.
	h3 := buildHistory(day("2026-09-01"), "In progress", []statusChangeAt{
		{day("2026-09-02"), "New", "Closed"}, {day("2026-09-04"), "Closed", "In progress"},
	}, byName)
	if !h3.Closed.IsZero() {
		t.Errorf("a reopened WP has no close, got %v", h3.Closed)
	}

	start := map[string]bool{"In progress": true, "In testing": true}
	r := flowReport([]wpHistory{h2, h1}, start, statuses, "p", "2026-09-02", "2026-09-15", time.UTC)
	if r["throughput"] != 2 {
		t.Errorf("throughput %v", r["throughput"])
	}
	ct := r["cycleTime"].(map[string]any)
	if ct["count"] != 1 || ct["p50"] != 4.0 {
		t.Errorf("cycle time %v, want one of 4 days", ct)
	}
	lt := r["leadTime"].(map[string]any)
	if lt["count"] != 2 || lt["max"] != 6.0 {
		t.Errorf("lead time %v", lt)
	}
	st := fmt.Sprint(r["statuses"])
	if !strings.Contains(st, "name:In progress perWP:map[count:1 max:3 p50:3 p85:3] share:75") || !strings.Contains(st, "In testing") || strings.Contains(st, "New") {
		t.Errorf("per status %s", st)
	}
	weeks := r["weekly"].([]map[string]any)
	if len(weeks) != 3 || weeks[0]["week"] != "2026-08-31" || weeks[0]["partial"] != true || weeks[1]["count"] != 1 || weeks[2]["count"] != 1 || weeks[2]["partial"] != true {
		t.Errorf("weekly %v", weeks)
	}
	wp1 := r["workPackages"].([]map[string]any)[0]
	hist := wp1["history"].([]map[string]any)
	if wp1["id"] != 1 || wp1["started"] != "2026-09-03T00:00:00Z" || len(hist) != 5 || hist[0]["days"] != 2.0 || hist[4]["left"] != nil {
		t.Errorf("wp 1 %v", wp1)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"30d": "2026-09-14", "2w": "2026-09-30", "2026-01-01": "2026-01-01", "-7d": "2026-10-07", "month": "2026-10-01"} {
		if got, err := parseSince(in, now); err != nil || got != want {
			t.Errorf("parseSince(%q) = %q, %v; want %s", in, got, err, want)
		}
	}
}

func TestListDateFlagsBuildFilters(t *testing.T) {
	env, f := newTestEnv(t)
	_, err := ByCmd("wp list").Execute(context.Background(), env, Args{"project": "all", "status": "all", "updated_after": "2026-09-01", "due_before": "2026-10-31"})
	if err != nil {
		t.Fatal(err)
	}
	var q string
	for _, r := range f.requests {
		if strings.HasPrefix(r, "GET /api/v3/work_packages?") {
			q = r
		}
	}
	filters, _ := url.ParseQuery(strings.SplitN(q, "?", 2)[1])
	got := filters.Get("filters")
	for _, want := range []string{`{"updatedAt":{"operator":"<>d","values":["2026-09-01",""]}}`, `{"dueDate":{"operator":"<>d","values":["","2026-10-31"]}}`} {
		if !strings.Contains(got, want) {
			t.Errorf("filters %s lack %s", got, want)
		}
	}
	if _, err := ByCmd("wp list").Execute(context.Background(), env, Args{"closed_after": "week", "page": 2}); err == nil {
		t.Error("--page with --closed-after must fail")
	}
}

func TestFullReadPagesByIDAndSortsLocally(t *testing.T) {
	items := []map[string]any{
		{"id": 1, "updatedAt": "2026-10-01T10:00:00.000Z"},
		{"id": 2, "updatedAt": "2026-10-03T10:00:00.000Z", "dueDate": "2026-11-01"},
		{"id": 3, "updatedAt": "2026-10-02T10:00:00.000Z", "dueDate": "2026-10-20"},
	}
	sortWPs(items, "updatedAt:desc")
	if got := fmt.Sprint(items[0]["id"], items[1]["id"], items[2]["id"]); got != "2 3 1" {
		t.Errorf("updatedAt:desc gave %s", got)
	}
	sortWPs(items, "dueDate:asc,id")
	if got := fmt.Sprint(items[0]["id"], items[1]["id"], items[2]["id"]); got != "3 2 1" {
		t.Errorf("dueDate:asc (empty last) gave %s", got)
	}
	if locallySortable("priority:desc") || !locallySortable("updatedAt:desc,id") {
		t.Error("locallySortable")
	}

	env, f := newTestEnv(t)
	if _, err := ByCmd("wp list").Execute(context.Background(), env, Args{"project": "all", "max": 0, "sort": "updatedAt:desc"}); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.requests {
		if strings.HasPrefix(r, "GET /api/v3/work_packages?") && !strings.Contains(r, url.QueryEscape(stableSort)) {
			t.Errorf("full read not paged in creation order: %s", r)
		}
	}
}
