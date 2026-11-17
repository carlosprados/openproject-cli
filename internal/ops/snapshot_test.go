package ops

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildSnapshotTreeAndStrayProjects(t *testing.T) {
	projects := []map[string]any{
		{"id": 10, "identifier": "ROOT", "name": "Root", "status": "On track", "active": true},
		{"id": 30, "identifier": "B", "name": "Offer B", "parent": "Root", "parentId": 10},
		{"id": 20, "identifier": "A", "name": "Offer A", "parent": "Root", "parentId": 10},
		{"id": 40, "identifier": "A1", "name": "Phase 1", "parent": "Offer A", "parentId": 20},
		{"id": 50, "identifier": "STRAY", "name": "Stray"}, // in the ancestor filter, no parent link
	}
	s, skipped := buildSnapshot(10, projects, func(id string) string { return "https://op.example/projects/" + id })
	if len(skipped) != 1 || skipped[0] != "STRAY (#50)" {
		t.Fatalf("skipped = %v", skipped)
	}
	var got []string
	for _, p := range s.Projects {
		b, _ := json.Marshal(p.Path)
		got = append(got, string(b))
	}
	want := `["ROOT"] ["ROOT","A"] ["ROOT","B"] ["ROOT","A","A1"]`
	if g := strings.Join(got, " "); g != want {
		t.Fatalf("paths by id = %s, want %s", g, want)
	}
	if s.Root != "ROOT" || s.Projects[0].Parent != "" || s.Projects[3].ParentID != 20 {
		t.Fatalf("snapshot = %+v", s)
	}

	s.Projects[1].Versions = []snapshotVersion{{ID: 3, Name: "v1", Status: "open", Sharing: "descendants", EndDate: "2026-12-01"}}
	var buf bytes.Buffer
	if err := renderSnapshot(&buf, s); err != nil {
		t.Fatal(err)
	}
	wantText := "ROOT  Root [On track]\n  A  Offer A\n    · v1 (open, descendants)  - → 2026-12-01\n    A1  Phase 1\n  B  Offer B\n"
	if buf.String() != wantText {
		t.Fatalf("text:\n%s\nwant:\n%s", buf.String(), wantText)
	}
}
