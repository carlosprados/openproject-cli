package ops

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Local files that travel with a write: --attach files and images that
// Markdown text references by a local path. OpenProject stores Markdown as
// is, so an image only renders if it points at an attachment of the work
// package; opcli uploads it and rewrites the link.

// FileDirKey is the Args key under which the CLI stores the directory of the
// file a text param was read from, so relative image paths resolve against
// the Markdown file rather than the working directory.
func FileDirKey(param string) string { return "@dir:" + param }

var pAttach = Param{Name: "attach", Kind: Strings, Desc: "Local file to attach (repeatable); a file already attached with the same name and size is not uploaded again"}

// mdImage matches ![alt](target "title"); the target is group 2.
var mdImage = regexp.MustCompile(`!\[[^\]]*\]\(\s*(<([^>]+)>|([^)\s]+))(\s+"[^"]*")?\s*\)`)

type imageRef struct {
	start, end int    // byte range of the target in the text
	path       string // absolute local path
}

// localImages finds images with a local target outside code. Remote URLs,
// data URIs and absolute paths that do not exist locally (server paths such
// as /api/v3/attachments/1/content) are left alone; a missing relative path
// is an error.
func localImages(text, baseDir string) ([]imageRef, error) {
	masked := maskCode(text)
	var out []imageRef
	for _, m := range mdImage.FindAllStringSubmatchIndex(masked, -1) {
		s, e := m[6], m[7]
		if m[4] >= 0 {
			s, e = m[4], m[5]
		}
		target := text[s:e]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "data:") ||
			strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		p := target
		if u, err := url.PathUnescape(target); err == nil {
			p = u
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		} else if _, err := os.Stat(p); err != nil {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(abs); err != nil || !st.Mode().IsRegular() {
			return nil, fmt.Errorf("image %q not found (looked for %s)", target, abs)
		}
		out = append(out, imageRef{start: s, end: e, path: abs})
	}
	return out, nil
}

// maskCode blanks fenced blocks and inline code spans, keeping byte offsets.
func maskCode(text string) string {
	b := []byte(text)
	fence := ""
	pos := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trim := strings.TrimLeft(line, " ")
		marker := ""
		if len(line)-len(trim) <= 3 {
			for _, f := range []string{"```", "~~~"} {
				if strings.HasPrefix(trim, f) {
					marker = f
				}
			}
		}
		switch {
		case fence != "":
			blank(b[pos : pos+len(line)])
			if marker == fence {
				fence = ""
			}
		case marker != "":
			fence = marker
			blank(b[pos : pos+len(line)])
		default:
			maskInline(b[pos : pos+len(line)])
		}
		pos += len(line)
	}
	return string(b)
}

func maskInline(line []byte) {
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		n := 1
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		closing := strings.Index(string(line[i+n:]), strings.Repeat("`", n))
		if closing < 0 {
			i += n - 1
			continue
		}
		end := i + n + closing + n
		blank(line[i:end])
		i = end - 1
	}
}

func blank(b []byte) {
	for i := range b {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

// attacher uploads the files of one write. With wpID == 0 the files are
// uploaded without a container and claimed by the work package create.
type attacher struct {
	e        *Env
	wpID     int
	existing map[string]map[string]any // attached files by name, loaded lazily
	ids      map[string]int            // uploaded or reused, by local path
	Uploaded []map[string]any
}

func (e *Env) newAttacher(wpID int) *attacher {
	return &attacher{e: e, wpID: wpID, ids: map[string]int{}}
}

// checkFiles verifies every file before anything is written: it exists and
// no two different files share a name (they would collide on the server).
func checkFiles(paths []string) error {
	byName := map[string]string{}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("file %s not found", p)
		}
		name := filepath.Base(p)
		if other, ok := byName[name]; ok && other != p {
			return fmt.Errorf("%s and %s have the same name: rename one", other, p)
		}
		byName[name] = p
	}
	return nil
}

// upload sends path once per write, reusing an attachment of the work
// package with the same name and size.
func (t *attacher) upload(ctx context.Context, path string) (int, error) {
	if id, ok := t.ids[path]; ok {
		return id, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	name := filepath.Base(path)
	if t.wpID > 0 {
		if t.existing == nil {
			items, _, err := t.e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/attachments", t.wpID), nil, 0)
			if err != nil {
				return 0, err
			}
			t.existing = map[string]map[string]any{}
			for _, it := range items {
				t.existing[hal.String(it["fileName"])] = it
			}
		}
		if cur, ok := t.existing[name]; ok && int64(intOf(cur["fileSize"])) == st.Size() {
			id := intOf(cur["id"])
			t.ids[path] = id
			t.Uploaded = append(t.Uploaded, map[string]any{"id": id, "fileName": name, "reused": true})
			return id, nil
		}
	}
	target := "/attachments"
	if t.wpID > 0 {
		target = fmt.Sprintf("/work_packages/%d/attachments", t.wpID)
	}
	res, err := t.e.C.Upload(ctx, target, path, "")
	if err != nil {
		return 0, fmt.Errorf("uploading %s: %w", path, err)
	}
	id := intOf(res["id"])
	t.ids[path] = id
	t.Uploaded = append(t.Uploaded, map[string]any{"id": id, "fileName": name})
	return id, nil
}

// rewrite uploads the images of text and points them at the attachments.
func (t *attacher) rewrite(ctx context.Context, text string, imgs []imageRef) (string, error) {
	ids := make([]int, len(imgs))
	for i, im := range imgs { // in document order
		id, err := t.upload(ctx, im.path)
		if err != nil {
			return "", err
		}
		ids[i] = id
	}
	for i := len(imgs) - 1; i >= 0; i-- { // from the end, so offsets stay valid
		text = text[:imgs[i].start] + fmt.Sprintf("%s/attachments/%d/content", client.APIPrefix, ids[i]) + text[imgs[i].end:]
	}
	return text, nil
}

// discard deletes what a failed create uploaded without a container. Best
// effort: OpenProject also purges unclaimed attachments every night.
func (t *attacher) discard(ctx context.Context) {
	if t.wpID > 0 {
		return
	}
	for _, u := range t.Uploaded {
		_ = t.e.C.Delete(ctx, fmt.Sprintf("/attachments/%d", intOf(u["id"])))
	}
}

// claims returns the _links.attachments of a create for the uploaded files.
func (t *attacher) claims() []any {
	out := make([]any, 0, len(t.Uploaded))
	for _, u := range t.Uploaded {
		out = append(out, hal.Ref(href("attachments", intOf(u["id"]))))
	}
	return out
}

// textFiles collects the local images of the Markdown params and the
// --attach files of a, and checks them all before any write.
type textFiles struct {
	images map[string][]imageRef // by param
	attach []string
}

func collectFiles(a Args, params ...string) (*textFiles, error) {
	tf := &textFiles{images: map[string][]imageRef{}}
	var all []string
	for _, p := range params {
		if !a.Has(p) {
			continue
		}
		dir, _ := a[FileDirKey(p)].(string)
		if dir == "" {
			dir = "."
		}
		imgs, err := localImages(a.String(p), dir)
		if err != nil {
			return nil, err
		}
		tf.images[p] = imgs
		for _, im := range imgs {
			all = append(all, im.path)
		}
	}
	for _, f := range rawStrings(a["attach"]) {
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		tf.attach = append(tf.attach, abs)
		all = append(all, abs)
	}
	if err := checkFiles(all); err != nil {
		return nil, err
	}
	return tf, nil
}

func (tf *textFiles) empty() bool {
	for _, imgs := range tf.images {
		if len(imgs) > 0 {
			return false
		}
	}
	return len(tf.attach) == 0
}

// apply uploads everything through t and returns the param values with
// their images rewritten.
func (tf *textFiles) apply(ctx context.Context, t *attacher, a Args) (map[string]string, error) {
	out := map[string]string{}
	for p, imgs := range tf.images {
		s, err := t.rewrite(ctx, a.String(p), imgs)
		if err != nil {
			return nil, err
		}
		out[p] = s
	}
	for _, f := range tf.attach {
		if _, err := t.upload(ctx, f); err != nil {
			return nil, err
		}
	}
	return out, nil
}
