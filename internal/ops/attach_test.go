package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLocalImagesSkipsCodeAndRemote(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.png": "x", "sub/b c.png": "y"})
	text := "![a](a.png) `![no](gone.png)`\n" +
		"```\n![no](gone.png)\n```\n" +
		"![r](https://example.com/r.png) ![s](/api/v3/attachments/1/content)\n" +
		"![b](<sub/b c.png> \"title\") ![b2](sub/b%20c.png)"
	imgs, err := localImages(text, dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, im := range imgs {
		got = append(got, text[im.start:im.end]+"="+filepath.Base(im.path))
	}
	want := []string{"a.png=a.png", "sub/b c.png=b c.png", "sub/b%20c.png=b c.png"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := localImages("![x](missing.png)", dir); err == nil || !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("expected missing image error, got %v", err)
	}
}

func TestCheckFilesRejectsNameClash(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a/x.png": "1", "b/x.png": "2"})
	a, b := filepath.Join(dir, "a/x.png"), filepath.Join(dir, "b/x.png")
	if err := checkFiles([]string{a, a}); err != nil {
		t.Errorf("same file twice must pass: %v", err)
	}
	if err := checkFiles([]string{a, b}); err == nil {
		t.Error("expected a name clash error")
	}
}

// A comment with images reuses an attachment with the same name and size,
// uploads the rest to the work package and leaves code alone.
func TestCommentUploadsAndRewritesImages(t *testing.T) {
	dir := writeFiles(t, map[string]string{"shot.png": "abc", "img/new.png": "12345"})
	env, f := newTestEnv(t)
	body := "Before ![a](shot.png) after ![b](img/new.png) and `![c](shot.png)`"
	res, err := ByCmd("wp comment").Execute(context.Background(), env, Args{"id": "42", "body": body, FileDirKey("body"): dir})
	if err != nil {
		t.Fatal(err)
	}
	var sent string
	uploads := 0
	for i, r := range f.requests {
		switch {
		case strings.HasPrefix(r, "POST /api/v3/work_packages/42/activities"):
			sent = f.bodies[i]["comment"].(map[string]any)["raw"].(string)
		case strings.HasPrefix(r, "POST /api/v3/work_packages/42/attachments"):
			uploads++
		}
	}
	if uploads != 1 {
		t.Errorf("uploads = %d, want 1 (shot.png is already attached)", uploads)
	}
	if !strings.Contains(sent, "![a](/api/v3/attachments/7/content)") ||
		!strings.Contains(sent, "](/api/v3/attachments/5") || !strings.Contains(sent, "`![c](shot.png)`") {
		t.Errorf("comment not rewritten as expected: %s", sent)
	}
	up := res.(map[string]any)["uploaded"].([]map[string]any)
	if len(up) != 2 || up[0]["reused"] != true || up[1]["fileName"] != "new.png" {
		t.Errorf("uploaded = %v", up)
	}
}

func TestMissingImageWritesNothing(t *testing.T) {
	env, f := newTestEnv(t)
	_, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "42", "status": "New", "comment": "![x](nope.png)"})
	if err == nil {
		t.Fatal("expected an error for a missing image")
	}
	if len(f.requests) != 0 {
		t.Errorf("requests sent before failing: %v", f.requests)
	}
}

// On create, files go up without a container and are claimed by the POST.
func TestCreateClaimsUploadedFiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{"d.png": "x", "log.txt": "y"})
	env, f := newTestEnv(t)
	_, err := ByCmd("wp create").Execute(context.Background(), env, Args{
		"project": "4", "type": "Task", "subject": "New",
		"description": "![d](d.png)", FileDirKey("description"): dir,
		"attach": []string{filepath.Join(dir, "log.txt")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var create map[string]any
	containerless := 0
	for i, r := range f.requests {
		switch {
		case strings.HasPrefix(r, "POST /api/v3/attachments"):
			containerless++
		case strings.HasPrefix(r, "POST /api/v3/work_packages?"):
			create = f.bodies[i]
		}
	}
	if containerless != 2 || create == nil {
		t.Fatalf("requests: %v", f.requests)
	}
	claims := create["_links"].(map[string]any)["attachments"].([]any)
	desc := create["description"].(map[string]any)["raw"].(string)
	if len(claims) != 2 || !strings.HasPrefix(desc, "![d](/api/v3/attachments/") {
		t.Errorf("claims %v, description %q", claims, desc)
	}
}

// A rejected create deletes the files it uploaded without a container.
func TestFailedCreateDiscardsUploads(t *testing.T) {
	dir := writeFiles(t, map[string]string{"d.png": "x"})
	env, f := newTestEnv(t)
	_, err := ByCmd("wp create").Execute(context.Background(), env, Args{
		"project": "4", "type": "Task", "subject": "fail",
		"description": "![d](d.png)", FileDirKey("description"): dir,
	})
	if err == nil {
		t.Fatal("expected the create to fail")
	}
	deleted := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, "DELETE /api/v3/attachments/") {
			deleted++
		}
	}
	if deleted != 1 {
		t.Errorf("deleted %d uploads, want 1: %v", deleted, f.requests)
	}
}
