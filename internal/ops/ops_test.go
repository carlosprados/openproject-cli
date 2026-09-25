package ops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
)

// TestRegistryConventions guards the CLI ↔ MCP contract: unique tool names
// and CLI paths, snake_case params, no flag collisions, help on every op.
func TestRegistryConventions(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	tools := map[string]bool{}
	paths := map[string]bool{}
	groups := map[string]bool{"": true}
	for _, g := range Groups() {
		groups[g.Name] = true
	}
	reserved := map[string]bool{"help": true, "output": true, "config": true, "url": true, "api-key": true}
	reservedShort := map[string]bool{"h": true, "o": true, "v": true}

	for _, op := range All() {
		if !strings.HasPrefix(op.Tool, "op_") || !snake.MatchString(op.Tool) {
			t.Errorf("%s: tool name must be op_snake_case", op.Tool)
		}
		if tools[op.Tool] {
			t.Errorf("duplicate tool %s", op.Tool)
		}
		tools[op.Tool] = true
		path := strings.Join(op.Cmd, " ")
		if paths[path] {
			t.Errorf("duplicate command %q", path)
		}
		paths[path] = true
		if !groups[strings.Join(op.Cmd[:len(op.Cmd)-1], " ")] {
			t.Errorf("%s: parent group of %q not registered", op.Tool, path)
		}
		if op.Short == "" || op.Example == "" || op.Run == nil {
			t.Errorf("%s: Short, Example and Run are mandatory", op.Tool)
		}

		flags := map[string]bool{}
		shorts := map[string]bool{}
		positionals := 0
		for i, p := range op.Params {
			if !snake.MatchString(p.Name) {
				t.Errorf("%s: param %q is not snake_case", op.Tool, p.Name)
			}
			for _, f := range []string{p.Flag(), p.Flag() + "-file"} {
				if f == p.Flag()+"-file" && !p.File {
					continue
				}
				if flags[f] || reserved[f] {
					t.Errorf("%s: flag --%s collides", op.Tool, f)
				}
				flags[f] = true
			}
			if p.Short != "" {
				if shorts[p.Short] || reservedShort[p.Short] {
					t.Errorf("%s: shorthand -%s collides", op.Tool, p.Short)
				}
				shorts[p.Short] = true
			}
			if p.Positional {
				positionals++
				if p.Rest && i != len(op.Params)-1 && op.Params[i+1].Positional {
					t.Errorf("%s: Rest param must be the last positional", op.Tool)
				}
			}
			if p.Default != nil {
				ok := false
				switch p.Kind {
				case String:
					_, ok = p.Default.(string)
				case Int:
					_, ok = p.Default.(int)
				case Bool:
					_, ok = p.Default.(bool)
				}
				if !ok {
					t.Errorf("%s: default of %s has the wrong type", op.Tool, p.Name)
				}
			}
		}
		if positionals > 2 {
			t.Errorf("%s: too many positionals", op.Tool)
		}
	}
}

func TestArgs(t *testing.T) {
	a := Args{"n": float64(3), "s": " x ", "list": []any{"a, b", "c"}, "csv": "d,e", "b": "true"}
	if a.Int("n") != 3 || a.String("n") != "3" || a.String("s") != "x" || !a.Bool("b") {
		t.Errorf("scalar coercion failed: %v", a)
	}
	if got := a.Strings("list"); strings.Join(got, "|") != "a|b|c" {
		t.Errorf("Strings(list) = %v", got)
	}
	if got := a.Strings("csv"); strings.Join(got, "|") != "d|e" {
		t.Errorf("Strings(csv) = %v", got)
	}
}

func TestParseDate(t *testing.T) {
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC) // Friday
	cases := map[string]string{
		"today": "2026-09-25", "yesterday": "2026-09-24", "tomorrow": "2026-09-26",
		"week": "2026-09-21", "month": "2026-09-01", "-7d": "2026-09-18", "+3d": "2026-09-28",
		"2026-01-02": "2026-01-02", "": "",
	}
	for in, want := range cases {
		got, err := parseDateAt(in, now)
		if err != nil || got != want {
			t.Errorf("parseDateAt(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := parseDateAt("next friday", now); err == nil {
		t.Error("expected error")
	}
}

func TestSortByAndFilters(t *testing.T) {
	got, err := sortBy("updatedAt:desc, id")
	if err != nil || got != `[["updatedAt","desc"],["id","asc"]]` {
		t.Errorf("sortBy = %s, %v", got, err)
	}
	if _, err := sortBy("id:sideways"); err == nil {
		t.Error("expected invalid direction error")
	}
	f, err := NewFilters().Add("status", "o").Add("project", "=", 4).Merge(`[{"dueDate":{"operator":"<t+","values":["7"]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"status":{"operator":"o","values":[]}},{"project":{"operator":"=","values":["4"]}},{"dueDate":{"operator":"<t+","values":["7"]}}]`
	if f.JSON() != want {
		t.Errorf("filters = %s", f.JSON())
	}
	if _, err := f.Merge("{bad"); err == nil {
		t.Error("expected JSON error")
	}
}

func TestPick(t *testing.T) {
	items := []named{{1, "New"}, {7, "In progress"}, {12, "Closed"}, {13, "On hold"}, {2, "In specification"}}
	for ref, want := range map[string]int{"closed": 12, "progress": 7, "7": 7, "#13": 13} {
		if got, err := pick("status", ref, items); err != nil || got != want {
			t.Errorf("pick(%q) = %d, %v; want %d", ref, got, err, want)
		}
	}
	if _, err := pick("status", "in", items); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguity error, got %v", err)
	}
	if _, err := pick("status", "nope", items); err == nil || !strings.Contains(err.Error(), "In progress (7)") {
		t.Errorf("expected error listing values, got %v", err)
	}
}

// fakeOP serves a minimal OpenProject for handler tests and records requests.
type fakeOP struct {
	t        *testing.T
	requests []string
	bodies   []map[string]any
}

func (f *fakeOP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.bodies = append(f.bodies, body)
	w.Header().Set("Content-Type", "application/hal+json")
	switch {
	case r.URL.Path == "/api/v3/groups/5":
		_, _ = w.Write([]byte(`{"_type":"Group","id":5,"name":"Devs"}`))
	case r.URL.Path == "/api/v3/statuses":
		_, _ = w.Write([]byte(`{"_type":"Collection","total":2,"count":2,"_embedded":{"elements":[{"id":1,"name":"New"},{"id":7,"name":"In progress"}]}}`))
	case r.URL.Path == "/api/v3/work_packages/42" && r.Method == "GET":
		_, _ = w.Write([]byte(`{"_type":"WorkPackage","id":42,"lockVersion":5,"subject":"S","_links":{"project":{"href":"/api/v3/projects/4","title":"P"}}}`))
	case r.URL.Path == "/api/v3/work_packages/42" && r.Method == "PATCH":
		if body["lockVersion"] != float64(5) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"_type":"Error","message":"stale"}`))
			return
		}
		_, _ = w.Write([]byte(`{"_type":"WorkPackage","id":42,"lockVersion":6,"subject":"S","_links":{"status":{"href":"/api/v3/statuses/7","title":"In progress"}}}`))
	case r.URL.Path == "/api/v3/work_packages" && r.Method == "GET":
		_, _ = w.Write([]byte(`{"_type":"Collection","total":1,"count":1,"_embedded":{"elements":[{"id":1,"subject":"A","description":{"format":"markdown","raw":"long"},"_links":{"status":{"href":"/api/v3/statuses/1","title":"New"}}}]}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"_type":"Error","errorIdentifier":"urn:openproject-org:api:v3:errors:NotFound","message":"not found"}`))
	}
}

func newTestEnv(t *testing.T) (*Env, *fakeOP) {
	f := &fakeOP{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewEnv(client.New(srv.URL, "k"), ""), f
}

func TestUpdateWorkPackageSendsLockVersionAndResolvedStatus(t *testing.T) {
	env, f := newTestEnv(t)
	res, err := ByTool("op_update_work_package").Execute(context.Background(), env, Args{"id": "#42", "status": "in progress"})
	if err != nil {
		t.Fatal(err)
	}
	wp := res.(map[string]any)
	if wp["status"] != "In progress" || !strings.HasSuffix(wp["url"].(string), "/work_packages/42") {
		t.Errorf("unexpected result %v", wp)
	}
	var patch map[string]any
	for i, r := range f.requests {
		if strings.HasPrefix(r, "PATCH") {
			patch = f.bodies[i]
		}
	}
	links := patch["_links"].(map[string]any)
	if patch["lockVersion"] != float64(5) || links["status"].(map[string]any)["href"] != "/api/v3/statuses/7" {
		t.Errorf("bad PATCH body %v", patch)
	}
	if _, ok := patch["notify"]; ok {
		t.Error("notify must be a query param, not a body field")
	}
}

func TestListWorkPackagesBuildsFiltersAndCompacts(t *testing.T) {
	env, f := newTestEnv(t)
	res, err := ByTool("op_list_work_packages").Execute(context.Background(), env, Args{"project": "all", "assignee": "me", "status": "closed"})
	if err != nil {
		t.Fatal(err)
	}
	l := res.(*List)
	if l.Total != 1 || l.Items[0]["status"] != "New" {
		t.Errorf("unexpected list %+v", l)
	}
	if _, ok := l.Items[0]["description"]; ok {
		t.Error("description should be dropped from listings")
	}
	q := f.requests[len(f.requests)-1]
	for _, want := range []string{`%22status%22%3A%7B%22operator%22%3A%22c%22`, `%22assignee%22%3A%7B%22operator%22%3A%22%3D%22%2C%22values%22%3A%5B%22me%22%5D`} {
		if !strings.Contains(q, want) {
			t.Errorf("query %s lacks %s", q, want)
		}
	}
}

func TestValidation(t *testing.T) {
	env, _ := newTestEnv(t)
	if _, err := ByTool("op_create_work_package").Execute(context.Background(), env, Args{}); err == nil || !strings.Contains(err.Error(), "subject") {
		t.Errorf("expected missing subject error, got %v", err)
	}
	if _, err := ByTool("op_create_relation").Execute(context.Background(), env, Args{"id": "1", "to": "2", "type": "bogus"}); err == nil || !strings.Contains(err.Error(), "allowed") {
		t.Errorf("expected enum error, got %v", err)
	}
	_, err := ByTool("op_get_work_package").Execute(context.Background(), env, Args{"id": "999"})
	if !client.IsNotFound(err) {
		t.Errorf("expected APIError 404, got %v", err)
	}
}

func TestReadOnlyAPIRequest(t *testing.T) {
	env, _ := newTestEnv(t)
	env.ReadOnly = true
	_, err := ByTool("op_api_request").Execute(context.Background(), env, Args{"method": "DELETE", "path": "work_packages/1"})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("expected read-only refusal, got %v", err)
	}
}

// Memberships need the typed principal href; /principals/:id is rejected.
func TestPrincipalHref(t *testing.T) {
	env, _ := newTestEnv(t)
	h, err := env.principalHref(context.Background(), 5)
	if err != nil || h != "/api/v3/groups/5" {
		t.Errorf("principalHref(5) = %q, %v; want /api/v3/groups/5", h, err)
	}
	if _, err := env.principalHref(context.Background(), 404); err == nil {
		t.Error("expected error for unknown principal")
	}
}
