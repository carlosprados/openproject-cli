package ops

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/carlosprados/openproject-cli/internal/client"
)

func TestPlaceCard(t *testing.T) {
	cases := []struct {
		name  string
		order []int
		pos   map[int]int
		at    int
		want  map[int]int
	}{
		{"empty column", nil, map[int]int{}, 0, map[int]int{9: 0}},
		{"top of an unpositioned column", []int{1, 2}, map[int]int{}, 0, map[int]int{9: 0}},
		{"top above a positioned card", []int{1, 2}, map[int]int{1: 0}, 0, map[int]int{9: -orderDistance}},
		{"between two positioned cards", []int{1, 2}, map[int]int{1: 0, 2: 100}, 1, map[int]int{9: 50}},
		{"after the last positioned card", []int{1, 2, 3}, map[int]int{1: 0}, 1, map[int]int{9: orderDistance}},
		{"bottom of a positioned column", []int{1, 2}, map[int]int{1: 0, 2: 10}, 2, map[int]int{9: 10 + orderDistance}},
		{"no room between neighbours", []int{1, 2, 3}, map[int]int{1: 0, 2: 1}, 1,
			map[int]int{9: orderDistance, 2: 2 * orderDistance}},
		{"among unpositioned cards: renumber the head", []int{1, 2, 3}, map[int]int{}, 2,
			map[int]int{1: 0, 2: orderDistance, 9: 2 * orderDistance}},
		{"bottom under unpositioned cards", []int{1, 2}, map[int]int{1: 0}, 2,
			map[int]int{2: orderDistance, 9: 2 * orderDistance}},
		{"never -1, which removes", []int{1, 2}, map[int]int{1: -3, 2: 1}, 1, map[int]int{9: -2}},
	}
	for _, c := range cases {
		if got := placeCard(c.order, c.pos, 9, c.at); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSortCards(t *testing.T) {
	items := []map[string]any{{"id": 5}, {"id": 3}, {"id": 8}, {"id": 1}, {"id": 4}}
	sortCards(items, map[int]int{8: 0, 4: -10, 5: 0})
	var got []int
	for _, it := range items {
		got = append(got, intOf(it["id"]))
	}
	if want := []int{4, 5, 8, 1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseBoard(t *testing.T) {
	w := func(id, start int, qid any, key, val string) map[string]any {
		return map[string]any{"id": id, "identifier": "work_package_query", "startColumn": start,
			"options": map[string]any{"queryId": qid, "filters": []any{map[string]any{key: map[string]any{"operator": "=", "values": []any{val}}}}}}
	}
	res := map[string]any{
		"id": 32, "name": "Kanban", "options": map[string]any{"type": "action", "attribute": "status"},
		"widgets": []any{w(145, 3, "81", "status", "16"), w(114, 1, 57, "status_id", "1"), w(146, 2, "82", "status", "7")},
		"_links":  map[string]any{"scope": map[string]any{"href": "/projects/AXI/boards"}},
	}
	b, err := parseBoard(res)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range b.Columns {
		got = append(got, c.Value)
	}
	if b.Project != "AXI" || b.Kind != "action" || !reflect.DeepEqual(got, []string{"1", "7", "16"}) || b.Columns[2].QueryID != 81 {
		t.Fatalf("board: %+v, values %v", b, got)
	}

	free, err := parseBoard(map[string]any{"id": 2, "options": map[string]any{"highlightingMode": "priority"},
		"_links": map[string]any{"scope": map[string]any{"href": "/projects/DP/boards"}}})
	if err != nil || free.Kind != "free" {
		t.Fatalf("free board: %+v %v", free, err)
	}
	if _, err := parseBoard(map[string]any{"id": 1, "_links": map[string]any{"scope": map[string]any{"href": "/my/page"}}}); err == nil {
		t.Fatal("a grid outside /projects/*/boards is not a board")
	}
}

func TestCardValue(t *testing.T) {
	wp := map[string]any{"_links": map[string]any{
		"status":         map[string]any{"href": "/api/v3/statuses/7"},
		"targetVersions": []any{map[string]any{"href": "/api/v3/versions/12"}},
		"parent":         map[string]any{"href": nil},
	}}
	if v := cardValue(wp, boardAttrs["status"]); v != "7" {
		t.Errorf("status: %q", v)
	}
	if v := cardValue(wp, boardAttrs["version"]); v != "12" {
		t.Errorf("version: %q", v)
	}
	if v := cardValue(wp, boardAttrs["subtasks"]); v != "" {
		t.Errorf("parent: %q", v)
	}
}

// A manually sorted query is served as one forced page by OpenProject; it
// must be read by id, page after page, and put back in manual order.
func TestRunSavedQueryManualOrder(t *testing.T) {
	const n = 1203
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/hal+json")
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/v3/queries/5/order":
			_, _ = w.Write([]byte(`{"1200": 0, "7": 5}`))
		case "/api/v3/queries/5":
			size, _ := strconv.Atoi(q.Get("pageSize"))
			off, _ := strconv.Atoi(q.Get("offset"))
			if q.Get("sortBy") == "" && size != 0 {
				size, off = 250, 1 // what the server forces on manual queries
			}
			var els []string
			for id := (off-1)*size + 1; id <= min(off*size, n) && size > 0; id++ {
				els = append(els, fmt.Sprintf(`{"_type":"WorkPackage","id":%d,"subject":"S"}`, id))
			}
			fmt.Fprintf(w, `{"_links":{"sortBy":[{"href":"/api/v3/queries/sort_bys/manualSorting-asc"},{"href":"/api/v3/queries/sort_bys/id-asc"}]},
				"_embedded":{"results":{"total":%d,"count":%d,"_embedded":{"elements":[%s]}}}}`, n, len(els), strings.Join(els, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	env := NewEnv(client.New(srv.URL, "k"), "")
	ids := func(l *List) []int {
		var out []int
		for _, it := range l.Items {
			out = append(out, intOf(it["id"]))
		}
		return out
	}

	l, err := env.runSavedQuery(context.Background(), 5, Args{"max": 0})
	if err != nil {
		t.Fatal(err)
	}
	if l.Total != n || len(l.Items) != n || !reflect.DeepEqual(ids(l)[:4], []int{1200, 7, 1, 2}) {
		t.Fatalf("all: total %d, %d items, head %v", l.Total, len(l.Items), ids(l)[:4])
	}
	l, err = env.runSavedQuery(context.Background(), 5, Args{"max": 3, "page": 2})
	if err != nil || !reflect.DeepEqual(ids(l), []int{2, 3, 4}) {
		t.Fatalf("page 2 of 3: %v %v", ids(l), err)
	}
	l, err = env.runSavedQuery(context.Background(), 5, Args{"max": 2})
	if err != nil || !reflect.DeepEqual(ids(l), []int{1200, 7}) {
		t.Fatalf("max 2: %v %v", ids(l), err)
	}
}
