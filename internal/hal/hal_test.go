package hal

import (
	"encoding/json"
	"reflect"
	"testing"
)

const wpJSON = `{
  "_type": "WorkPackage", "id": 42, "subject": "Fix login", "lockVersion": 3,
  "description": {"format": "markdown", "raw": "Some **text**", "html": "<p>Some</p>"},
  "estimatedTime": "PT2H",
  "_embedded": {"relations": {"_type": "Collection", "_embedded": {"elements": [
     {"_type": "Relation", "id": 9, "type": "relates", "_links": {"to": {"href": "/api/v3/work_packages/40", "title": "Other"}}}
  ]}}},
  "_links": {
    "self": {"href": "/api/v3/work_packages/42"},
    "status": {"href": "/api/v3/statuses/7", "title": "In progress"},
    "assignee": {"href": null},
    "user": {"href": "/api/v3/users/8"},
    "update": {"href": "/api/v3/work_packages/42", "method": "patch"},
    "copy": {"href": "/work_packages/42/copy", "title": "Copy"},
    "emojiReactions": {"href": "/api/v3/work_packages/42/emoji_reactions"},
    "projectStatus": {"href": "/api/v3/project_statuses/on_track", "title": "On track"},
    "targetVersions": [{"href": "/api/v3/versions/3", "title": "Sprint 1"}],
    "addWatcher": {"href": "/api/v3/work_packages/42/watchers", "method": "post", "templated": true}
  }
}`

func TestFlatten(t *testing.T) {
	var res map[string]any
	if err := json.Unmarshal([]byte(wpJSON), &res); err != nil {
		t.Fatal(err)
	}
	got := Flatten(res, "relations")

	want := map[string]any{
		"_type":             "WorkPackage",
		"id":                float64(42),
		"subject":           "Fix login",
		"lockVersion":       float64(3),
		"description":       "Some **text**",
		"estimatedTime":     "PT2H",
		"status":            "In progress",
		"statusId":          7,
		"assignee":          nil,
		"userId":            8,
		"projectStatus":     "On track",
		"projectStatusId":   "on_track",
		"targetVersions":    []any{"Sprint 1"},
		"targetVersionsIds": []any{3},
	}
	rels, _ := got["relations"].([]map[string]any)
	delete(got, "relations")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Flatten mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if len(rels) != 1 || rels[0]["toId"] != 40 || rels[0]["to"] != "Other" {
		t.Errorf("embedded relations not flattened: %#v", rels)
	}
}

func TestIDFromHref(t *testing.T) {
	cases := map[string]any{
		"/api/v3/users/8":                     8,
		"/api/v3/users/me":                    "me",
		"/api/v3/days/non_working/2026-01-01": "2026-01-01",
		"/api/v3/principals?filters=x":        "principals",
		"https://example.com/x":               nil,
		"":                                    nil,
	}
	for in, want := range cases {
		if got := IDFromHref(in); got != want {
			t.Errorf("IDFromHref(%q) = %#v, want %#v", in, got, want)
		}
	}
}

func TestParseHours(t *testing.T) {
	cases := map[string]float64{
		"1.5": 1.5, "1,5": 1.5, "90m": 1.5, "1h30m": 1.5, "1h 30m": 1.5,
		"2h": 2, "PT1H30M": 1.5, "pt45m": 0.75, "P1D": 24, "PT0S": 0,
	}
	for in, want := range cases {
		got, err := ParseHours(in)
		if err != nil || got != want {
			t.Errorf("ParseHours(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "P", "PT", "1x"} {
		if _, err := ParseHours(bad); err == nil {
			t.Errorf("ParseHours(%q) should fail", bad)
		}
	}
}

func TestISOHours(t *testing.T) {
	cases := map[float64]string{1.5: "PT1H30M", 2: "PT2H", 0.25: "PT15M", 0: "PT0S", 1.0 / 3: "PT20M"}
	for in, want := range cases {
		if got := ISOHours(in); got != want {
			t.Errorf("ISOHours(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestHoursString(t *testing.T) {
	if got := HoursString("PT2H30M"); got != "2.5h" {
		t.Errorf("got %q", got)
	}
	if got := HoursString(nil); got != "" {
		t.Errorf("nil → %q", got)
	}
}
