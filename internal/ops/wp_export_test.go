package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testExporter(t *testing.T, a Args) *exporter {
	t.Helper()
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("no tzdata")
	}
	return &exporter{e: &Env{}, a: a, byID: a.String("name") == "id", wikilink: a.String("links") == "wikilink",
		loc: madrid, base: "https://op.example", lang: "es"}
}

var exportWP = map[string]any{
	"id": 42, "subject": "Fix: login / SSO", "status": "Iniciado", "statusIsClosed": false,
	"parent": "Epic SSO", "parentId": 7, "startDate": "2026-10-28", "estimatedTime": "PT1H30M",
	"percentageDone": 40, "targetVersions": []any{"v1.2"}, "createdAt": "2026-11-13T10:33:31.112Z",
	"customFields": map[string]any{"Clave externa": "EXT-1", "Tags": []any{"a", "b"}},
	"description":  `Ver ![x](/api/v3/attachments/5/content) y <mention class="mention" data-id="3" data-type="user" data-text="@Ana">@Ana L&oacute;pez</mention>.`,
}

func TestExportRenderWithMap(t *testing.T) {
	x := testExporter(t, Args{"name": "id", "links": "wikilink"})
	path := filepath.Join(t.TempDir(), "map.yaml")
	os.WriteFile(path, []byte("set: {tipo: wp, generado: true}\nfields:\n  id: id\n  subject: asunto\n  version: version\n  parent: padre\n  startDate: inicio\n  estimatedTime: horas\n  percentDone: avance\n  createdAt: fecha\n  customFields.Clave externa: clave\n  customFields.Tags: tags\n  dueDate: fin\n"), 0o644)
	if err := x.loadMap(path); err != nil {
		t.Fatal(err)
	}
	x.setConst("frente", "Acme")
	doc, err := x.render(exportWP, []exportComment{{ID: 9, At: "2026-11-16T17:40:00Z", User: "Ana", Text: "Hecho"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `---
tipo: wp
generado: true
frente: Acme
id: 42
asunto: 'Fix: login / SSO'
version: v1.2
padre: '[[7]]'
inicio: 2026-10-28
horas: 1.5
avance: 40
fecha: 2026-11-13T11:33
clave: EXT-1
tags:
  - a
  - b
---

Ver ![x](https://op.example/api/v3/attachments/5/content) y @Ana López.

## Comentarios

### 2026-11-16T18:40 · Ana

Hecho
`
	if got := string(doc.Content); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if doc.File != "42.md" {
		t.Fatalf("file = %s", doc.File)
	}
}

func TestExportDefaultFieldsAndSlug(t *testing.T) {
	x := testExporter(t, Args{"name": "slug"})
	doc, err := x.render(exportWP, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc.Content)
	for _, want := range []string{"subject: 'Fix: login / SSO'\n", "parent: Epic SSO\n", "version: v1.2\n", "customFields:\n  Clave externa: EXT-1\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if doc.File != "42-fix-login-sso.md" {
		t.Fatalf("file = %s", doc.File)
	}
}

func TestExportWriteRenamesAndReportsGone(t *testing.T) {
	dir := t.TempDir()
	x := testExporter(t, Args{"name": "slug"})
	docs := []exportDoc{{ID: 1, File: "1-old.md", Content: []byte("a")}, {ID: 2, File: "2-b.md", Content: []byte("b")}}
	if _, err := x.write(dir, docs, Args{}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "10-other.md"), []byte("not ours"), 0o644)

	res, err := x.write(dir, []exportDoc{{ID: 1, File: "1-new.md", Content: []byte("a")}}, Args{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Renamed) != 1 || res.Renamed[0].From != "1-old.md" || res.Files[0].Status != "renamed" {
		t.Fatalf("renamed = %+v files = %+v", res.Renamed, res.Files)
	}
	if len(res.Gone) != 1 || res.Gone[0] != 2 {
		t.Fatalf("gone = %v", res.Gone)
	}
	for _, f := range []string{"1-new.md", "2-b.md", "10-other.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "1-old.md")); err == nil {
		t.Error("1-old.md left behind")
	}

	res, err = x.write(dir, []exportDoc{{ID: 1, File: "1-new.md", Content: []byte("a")}}, Args{})
	if err != nil || res.Unchanged != 1 {
		t.Fatalf("second run: %+v %v", res, err)
	}
}

func TestExportListArgsByID(t *testing.T) {
	la, err := exportListArgs(context.Background(), &Env{}, Args{"ids": "42 #43", "filter": `[{"type":{"operator":"=","values":["1"]}}]`, "dir": "x", "comments": 2})
	if err != nil {
		t.Fatal(err)
	}
	if la["status"] != "all" || la["project"] != "all" || la["max"] != 0 || la.Has("dir") || la.Has("comments") {
		t.Fatalf("args = %v", la)
	}
	want := `[{"id":{"operator":"=","values":["42","43"]}},{"type":{"operator":"=","values":["1"]}}]`
	if la["filter"] != want {
		t.Fatalf("filter = %v", la["filter"])
	}
}

func TestExportDisplayIDAndLists(t *testing.T) {
	x := testExporter(t, Args{"name": "id", "links": "wikilink"})
	path := filepath.Join(t.TempDir(), "map.yaml")
	os.WriteFile(path, []byte("fields:\n  id: id\n  displayId: ref\n  parent: padre\nlists:\n  aliases: [displayId]\n  tags: [customFields.Tags, status]\n"), 0o644)
	if err := x.loadMap(path); err != nil {
		t.Fatal(err)
	}
	wp := map[string]any{"id": 42, "displayId": "DEMO-7", "parent": "Epic", "parentId": 7, "status": "New", "customFields": map[string]any{"Tags": []any{"a", "b"}}}
	doc, err := x.render(wp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nid: 42\nref: DEMO-7\npadre: '[[7]]'\naliases:\n  - DEMO-7\ntags:\n  - a\n  - b\n  - New\n---\n"
	if got := string(doc.Content); got != want || doc.File != "42.md" {
		t.Fatalf("file %s, got:\n%s\nwant:\n%s", doc.File, got, want)
	}
	delete(wp, "displayId")
	if doc, _ = x.render(wp, nil); strings.Contains(string(doc.Content), "aliases") {
		t.Fatalf("empty list written:\n%s", doc.Content)
	}
}
