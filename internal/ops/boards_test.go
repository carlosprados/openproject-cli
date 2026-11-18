package ops

import (
	"reflect"
	"testing"
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
