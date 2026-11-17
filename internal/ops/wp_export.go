package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() { register(wpExport) }

// exportManifest is DIR/.export.json: what the last export wrote, so the
// next one (or another tool) can tell which work packages left the filter.
const exportManifest = ".export.json"

// defaultExportFields is the frontmatter without --map, in this order.
var defaultExportFields = []string{
	"id", "subject", "type", "status", "statusIsClosed", "priority", "project",
	"assignee", "responsible", "author", "version", "parent", "startDate", "dueDate",
	"estimatedTime", "remainingTime", "spentTime", "percentageDone", "storyPoints",
	"createdAt", "updatedAt", "url", "customFields",
}

// exportAliases are accepted in --map for names the CLI uses elsewhere.
var exportAliases = map[string]string{"percentDone": "percentageDone", "targetVersions": "version"}

var exportDurations = map[string]bool{"estimatedTime": true, "remainingTime": true, "spentTime": true, "derivedEstimatedTime": true, "derivedRemainingTime": true}

var exportTimestamps = map[string]bool{"createdAt": true, "updatedAt": true}

func exportParams() []Param {
	ps := []Param{{Name: "ids", Kind: String, Positional: true, Rest: true, Desc: "Work package ids (instead of, or on top of, the filters)"}}
	for _, p := range wpList.Params {
		switch p.Name {
		case "page", "sort", "query_id":
			continue
		case "max":
			p = pMax(0)
			p.Desc = "Maximum work packages to export (0 = all)"
		case "status":
			p.Default = nil
			p.Desc = "open, closed, all, or comma-separated status names/ids (default: open; with ids, all)"
		}
		ps = append(ps, p)
	}
	return append(ps,
		Param{Name: "dir", Kind: String, Short: "d", Desc: "Directory to write one file per work package into (created if missing); without it, a single work package goes to stdout"},
		Param{Name: "name", Kind: String, Default: "slug", Enum: []string{"slug", "id"}, Desc: "File names: slug (<id>-<subject>.md) or id (<id>.md)"},
		Param{Name: "map", Kind: String, Desc: "YAML file choosing and renaming frontmatter fields (fields:) and adding constants (set:)"},
		Param{Name: "set", Kind: Strings, Desc: "Constant frontmatter property KEY=VALUE (repeatable; overrides the map's set:)"},
		Param{Name: "links", Kind: String, Default: "none", Enum: []string{"none", "wikilink"}, Desc: "wikilink writes parent as \"[[file]]\" (the parent's file name)"},
		Param{Name: "comments", Kind: Int, Desc: "Append the last N comments (-1 = all; one request per work package)"},
		Param{Name: "exclude_comment", Kind: Strings, Desc: "Activity ids of comments never to export (e.g. one that carries a secret)"},
		Param{Name: "attachments", Kind: String, Default: "images", Enum: []string{"images", "all", "none"}, Desc: "With --dir, download into DIR/attachments/: images (those the text shows), all (every attachment, listed at the end) or none (links stay on the server)"},
	)
}

var wpExport = &Op{
	Cmd:   []string{"wp", "export"},
	Short: "Export work packages as Markdown files with YAML frontmatter",
	Long: `Write work packages as Markdown: a YAML frontmatter with their fields and
the description as the body, optionally followed by the last comments. It
selects work packages with the same filters as 'wp list' (all of them by
default, not 50; open ones unless --status says otherwise), or by id.

With --dir it writes one file per work package, <id>-<subject>.md (or <id>.md
with --name id). The output is deterministic: a file whose work package did
not change is left untouched, and a work package whose subject changed has
its file renamed, never duplicated. Nothing is ever deleted: DIR/.export.json
lists what this export wrote (id, file, updatedAt and the filters used), and
the result names in "gone" the work packages of the previous export that
this one no longer includes (closed, moved, deleted). Without --dir, a
single work package is printed to stdout.

Frontmatter: ids and numbers stay numbers, dates are YYYY-MM-DD, createdAt
and updatedAt are local date and time (YYYY-MM-DDTHH:MM, in your
OpenProject time zone), durations are hours as a number, the version is
"version", empty fields are left out, custom fields go under customFields by
name. --map takes a YAML file:

  set: {kind: wp, generated: true}     # constants
  fields:                              # only these, in this order, renamed
    id: id
    subject: title
    status: state
    customFields.Customer: customer    # flattened to the top level

--set KEY=VALUE adds constants per run. The body is the description in
clean Markdown: the HTML the editor stores (tables, <br>, <p>, links,
images, <details>) is converted outside code, and mentions become @Name.

--attachments (with --dir): images (default) downloads the attachments the
description and comments show into DIR/attachments/<id>-<name> and links
them relatively; all also downloads every attachment of each work package
and lists them at the end; none leaves links to the server, which need a
session. Files already downloaded (same size) are not fetched again.

It is one way: editing the files changes nothing in OpenProject.`,
	Example: `  opcli wp export 42                                   # to stdout
  opcli wp export -p demo-project --status all --dir notes/
  opcli wp export -p programme --subprojects --status all --dir wp --name id --links wikilink
  opcli wp export -p demo-project --dir wp --map wp-map.yaml --set "team=Backend"
  opcli wp export -p demo-project --dir wp --comments 5 --exclude-comment 1234
  opcli wp export -p demo-project --dir wp --attachments all
  opcli wp export -p demo-project --dir wp -o json | jq '.gone'`,
	Params: exportParams(),
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		x, err := newExporter(ctx, e, a)
		if err != nil {
			return nil, err
		}
		la, err := exportListArgs(a)
		if err != nil {
			return nil, err
		}
		l, err := e.listWorkPackages(ctx, la, true)
		if err != nil {
			return nil, err
		}
		if a.String("dir") == "" {
			if l.Count != 1 {
				return nil, fmt.Errorf("%d work packages match: pass --dir to write one file each, or select a single one", l.Count)
			}
		}
		comments, err := x.comments(ctx, l.Items)
		if err != nil {
			return nil, err
		}
		downloaded := 0
		if dir := a.String("dir"); dir != "" && a.String("attachments") != "none" {
			if downloaded, err = x.downloadAttachments(ctx, dir, l.Items, comments); err != nil {
				return nil, err
			}
		}
		docs := make([]exportDoc, len(l.Items))
		for i, it := range l.Items {
			docs[i], err = x.render(it, comments[intOf(it["id"])])
			if err != nil {
				return nil, err
			}
		}
		if a.String("dir") == "" {
			return &exportStdout{ID: docs[0].ID, Markdown: string(docs[0].Content)}, nil
		}
		res, err := x.write(a.String("dir"), docs, la)
		if res != nil {
			res.Downloaded = downloaded
		}
		return res, err
	},
	Text: func(w io.Writer, v any) error {
		switch r := v.(type) {
		case *exportStdout:
			_, err := io.WriteString(w, r.Markdown)
			return err
		case *exportResult:
			fmt.Fprintf(w, "%s: %d work packages (%d created, %d updated, %d renamed, %d unchanged), %d attachments downloaded\n",
				r.Dir, r.Count, r.Created, r.Updated, len(r.Renamed), r.Unchanged, r.Downloaded)
			for _, rn := range r.Renamed {
				fmt.Fprintf(w, "  renamed #%d: %s -> %s\n", rn.ID, rn.From, rn.To)
			}
			if len(r.Gone) > 0 {
				fmt.Fprintf(w, "  no longer exported (files kept): %v\n", r.Gone)
			}
		}
		return nil
	},
}

type exportStdout struct {
	ID       int    `json:"id"`
	Markdown string `json:"markdown"`
}

type exportRename struct {
	ID   int    `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type exportFile struct {
	ID     int    `json:"id"`
	File   string `json:"file"`
	Status string `json:"status"` // created, updated, renamed, unchanged
}

type exportResult struct {
	Dir        string         `json:"dir"`
	Count      int            `json:"count"`
	Created    int            `json:"created"`
	Updated    int            `json:"updated"`
	Unchanged  int            `json:"unchanged"`
	Renamed    []exportRename `json:"renamed"`
	Gone       []int          `json:"gone"`
	Downloaded int            `json:"downloaded"`
	Files      []exportFile   `json:"files"`
}

type manifestEntry struct {
	ID        int    `json:"id"`
	File      string `json:"file"`
	UpdatedAt string `json:"updatedAt"`
}

type manifest struct {
	Filters      map[string]any  `json:"filters"`
	Name         string          `json:"name"`
	WorkPackages []manifestEntry `json:"workPackages"`
}

type exportDoc struct {
	ID        int
	File      string
	UpdatedAt string
	Content   []byte
}

type exportField struct{ src, key string }

type exporter struct {
	e        *Env
	a        Args
	fields   []exportField // nil: the default fields with their own names
	set      [][2]string
	byID     bool
	wikilink bool
	loc      *time.Location
	base     string
	lang     string
	// local maps attachment ids to their downloaded path relative to DIR;
	// listed holds every attachment of each work package (--attachments all).
	local  map[int]string
	listed map[int][]exportAttachment
}

type exportAttachment struct {
	ID   int
	Name string
}

func newExporter(ctx context.Context, e *Env, a Args) (*exporter, error) {
	x := &exporter{e: e, a: a, byID: a.String("name") == "id", wikilink: a.String("links") == "wikilink",
		loc: e.location(ctx), base: e.C.BaseURL, lang: e.Language}
	if p := a.String("map"); p != "" {
		if err := x.loadMap(p); err != nil {
			return nil, err
		}
	}
	for _, kv := range rawStrings(a["set"]) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("--set %q: want KEY=VALUE", kv)
		}
		x.setConst(strings.TrimSpace(k), v)
	}
	return x, nil
}

func (x *exporter) setConst(k, v string) {
	for i := range x.set {
		if x.set[i][0] == k {
			x.set[i][1] = v
			return
		}
	}
	x.set = append(x.set, [2]string{k, v})
}

// loadMap reads --map keeping the order of its keys.
func (x *exporter) loadMap(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("--map %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return fmt.Errorf("--map %s is empty", path)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("--map %s: want a mapping with fields: and set:", path)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i].Value, root.Content[i+1]
		if val.Kind != yaml.MappingNode {
			return fmt.Errorf("--map %s: %s must be a mapping", path, key)
		}
		switch key {
		case "fields":
			x.fields = []exportField{}
			for j := 0; j+1 < len(val.Content); j += 2 {
				src, dst := val.Content[j].Value, val.Content[j+1].Value
				if a, ok := exportAliases[src]; ok {
					src = a
				}
				if dst == "" {
					dst = src
				}
				x.fields = append(x.fields, exportField{src, dst})
			}
		case "set":
			for j := 0; j+1 < len(val.Content); j += 2 {
				x.setConst(val.Content[j].Value, val.Content[j+1].Value)
			}
		default:
			return fmt.Errorf("--map %s: unknown key %q (want fields, set)", path, key)
		}
	}
	return nil
}

// exportListArgs turns export args into wp list args: all matches, and ids
// as an id filter (any status, any project unless given).
func exportListArgs(a Args) (Args, error) {
	la := Args{}
	for k, v := range a {
		switch k {
		case "ids", "dir", "name", "map", "set", "links", "comments", "exclude_comment", "attachments":
		default:
			la[k] = v
		}
	}
	if !la.Has("max") {
		la["max"] = 0
	}
	if a.String("ids") == "" {
		if !la.Has("status") {
			la["status"] = "open"
		}
		return la, nil
	}
	ids, err := workPackageIDs(a.String("ids"))
	if err != nil {
		return nil, err
	}
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strconv.Itoa(id)
	}
	filters := []any{map[string]any{"id": map[string]any{"operator": "=", "values": values}}}
	if raw := a.String("filter"); raw != "" {
		var extra []any
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return nil, fmt.Errorf("--filter: %w", err)
		}
		filters = append(filters, extra...)
	}
	b, _ := json.Marshal(filters)
	la["filter"] = string(b)
	if !a.Has("status") {
		la["status"] = "all"
	}
	if !a.Has("project") {
		la["project"] = "all"
	}
	return la, nil
}

type exportComment struct {
	ID   int
	At   string
	User string
	Text string
}

// comments reads the last --comments comments of each work package, a few
// work packages at a time.
func (x *exporter) comments(ctx context.Context, items []map[string]any) (map[int][]exportComment, error) {
	n := x.a.Int("comments")
	out := map[int][]exportComment{}
	if n == 0 {
		return out, nil
	}
	skip := map[int]bool{}
	for _, s := range x.a.Strings("exclude_comment") {
		id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
		if err != nil {
			return nil, fmt.Errorf("--exclude-comment %q: want an activity id", s)
		}
		skip[id] = true
	}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		sem      = make(chan struct{}, 8)
	)
	for _, it := range items {
		id := intOf(it["id"])
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			acts, err := x.e.activities(ctx, id)
			var cs []exportComment
			for _, act := range acts {
				text, _ := act["comment"].(string)
				if strings.TrimSpace(text) == "" || skip[intOf(act["id"])] {
					continue
				}
				cs = append(cs, exportComment{ID: intOf(act["id"]), At: fmt.Sprint(act["createdAt"]), User: fmt.Sprint(act["user"]), Text: text})
			}
			if n > 0 && len(cs) > n {
				cs = cs[len(cs)-n:]
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("comments of #%d: %w", id, err)
			}
			out[id] = cs
		}()
	}
	wg.Wait()
	return out, firstErr
}

func (x *exporter) render(it map[string]any, comments []exportComment) (exportDoc, error) {
	id := intOf(it["id"])
	doc := exportDoc{ID: id, File: x.fileName(id, fmt.Sprint(it["subject"])), UpdatedAt: fmt.Sprint(it["updatedAt"])}
	fm := &yaml.Node{Kind: yaml.MappingNode}
	add := func(key string, v any) error {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return err
		}
		// yaml quotes "2026-10-28" to keep it a string; a date is wanted.
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" && dateOnlyRe.MatchString(n.Value) {
			n.Tag, n.Style = "!!timestamp", 0
		}
		fm.Content = append(fm.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &n)
		return nil
	}
	for _, kv := range x.set {
		if err := add(kv[0], yamlScalar(kv[1])); err != nil {
			return doc, err
		}
	}
	fields := x.fields
	if fields == nil {
		for _, f := range defaultExportFields {
			fields = append(fields, exportField{f, f})
		}
	}
	for _, f := range fields {
		v := x.value(it, f.src)
		if isEmpty(v) {
			continue
		}
		if err := add(f.key, v); err != nil {
			return doc, err
		}
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	if len(fm.Content) > 0 {
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		if err := enc.Encode(fm); err != nil {
			return doc, err
		}
		_ = enc.Close()
	}
	b.WriteString("---\n")
	if d := strings.TrimSpace(x.cleanText(fmt.Sprint(firstNonNil(it["description"], "")))); d != "" {
		b.WriteString("\n" + d + "\n")
	}
	if len(comments) > 0 {
		b.WriteString("\n## " + x.word("Comments") + "\n")
		for _, c := range comments {
			fmt.Fprintf(&b, "\n### %s · %s\n\n%s\n", x.localTime(c.At), c.User, strings.TrimSpace(x.cleanText(c.Text)))
		}
	}
	if atts := x.listed[id]; len(atts) > 0 {
		b.WriteString("\n## " + x.word("Attachments") + "\n\n")
		for _, at := range atts {
			fmt.Fprintf(&b, "- [%s](%s)\n", at.Name, x.attachmentLink(at.ID))
		}
	}
	doc.Content = b.Bytes()
	return doc, nil
}

// value is one frontmatter value: "customFields.NAME" reaches into custom
// fields, the rest are work package keys, converted for frontmatter.
func (x *exporter) value(it map[string]any, src string) any {
	if name, ok := strings.CutPrefix(src, "customFields."); ok {
		cf, _ := it["customFields"].(map[string]any)
		return cf[name]
	}
	switch src {
	case "version":
		if vs, ok := it["targetVersions"].([]any); ok && len(vs) == 1 {
			return vs[0]
		} else if ok {
			return vs
		}
		return it["version"]
	case "parent":
		if x.wikilink && it["parentId"] != nil {
			return "[[" + strings.TrimSuffix(x.fileName(intOf(it["parentId"]), fmt.Sprint(it["parent"])), ".md") + "]]"
		}
	}
	v := it[src]
	switch {
	case v == nil:
		return nil
	case exportDurations[src]:
		h, err := hal.ParseHours(fmt.Sprint(v))
		if err != nil {
			return v
		}
		return h
	case exportTimestamps[src]:
		return x.localTime(fmt.Sprint(v))
	case src == "id":
		return intOf(v)
	}
	return v
}

func (x *exporter) localTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.In(x.loc).Format("2006-01-02T15:04")
}

var (
	dateOnlyRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	mentionWrap  = regexp.MustCompile(`(?s)<mention\b[^>]*>(.*?)</mention>`)
	serverLinkRe = regexp.MustCompile(`(\]\(|\b(?:src|href)=["'])/(api/v3/|attachments/|work_packages/|projects/|wp/)`)
)

// cleanText turns mention tags into their text and server-relative links
// into absolute ones.
func (x *exporter) cleanText(s string) string {
	s = mentionWrap.ReplaceAllStringFunc(s, func(m string) string {
		return html.UnescapeString(mentionWrap.FindStringSubmatch(m)[1])
	})
	s = htmlToMarkdown(s)
	s = x.attachmentRe().ReplaceAllStringFunc(s, func(m string) string {
		id, _ := strconv.Atoi(x.attachmentRe().FindStringSubmatch(m)[1])
		return x.attachmentLink(id)
	})
	return serverLinkRe.ReplaceAllString(s, "${1}"+strings.TrimSuffix(x.base, "/")+"/${2}")
}

// attachmentRe matches a link to an attachment's content on this server,
// relative or absolute.
func (x *exporter) attachmentRe() *regexp.Regexp {
	return regexp.MustCompile(`(?:` + regexp.QuoteMeta(strings.TrimSuffix(x.base, "/")) + `)?/api/v3/attachments/(\d+)/content`)
}

// attachmentLink is the downloaded copy, else the server URL.
func (x *exporter) attachmentLink(id int) string {
	if p, ok := x.local[id]; ok {
		return p
	}
	return fmt.Sprintf("%s/api/v3/attachments/%d/content", strings.TrimSuffix(x.base, "/"), id)
}

func (x *exporter) word(en string) string {
	if x.lang == "es" {
		switch en {
		case "Comments":
			return "Comentarios"
		case "Attachments":
			return "Adjuntos"
		}
	}
	return en
}

func (x *exporter) fileName(id int, subject string) string {
	if x.byID {
		return fmt.Sprintf("%d.md", id)
	}
	if s := slug(subject); s != "" {
		return fmt.Sprintf("%d-%s.md", id, s)
	}
	return fmt.Sprintf("%d.md", id)
}

// slug keeps letters and digits (accents included), lower-cased, joined by
// dashes, at most 60 characters.
func slug(s string) string {
	var b strings.Builder
	dash := false
	n := 0
	for _, r := range strings.ToLower(s) {
		if n >= 60 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			n++
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
			n++
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// yamlScalar types a --set or set: value: numbers and booleans stay so.
func yamlScalar(s string) any {
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err == nil {
		switch v.(type) {
		case bool, int, float64:
			return v
		}
	}
	return s
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// write puts each document in dir, renaming a work package's previous file
// instead of leaving a second one, and records the manifest.
func (x *exporter) write(dir string, docs []exportDoc, filters Args) (*exportResult, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	prev := map[int]bool{}
	if data, err := os.ReadFile(filepath.Join(dir, exportManifest)); err == nil {
		var m manifest
		if json.Unmarshal(data, &m) == nil {
			for _, w := range m.WorkPackages {
				prev[w.ID] = true
			}
		}
	}
	res := &exportResult{Dir: dir, Count: len(docs), Renamed: []exportRename{}, Gone: []int{}, Files: []exportFile{}}
	m := manifest{Filters: map[string]any{}, Name: x.a.String("name")}
	for k, v := range filters {
		m.Filters[k] = v
	}
	seen := map[int]bool{}
	for _, d := range docs {
		seen[d.ID] = true
		target := filepath.Join(dir, d.File)
		status := "created"
		for _, old := range existingFiles(dir, d.ID) {
			if old == d.File {
				status = "updated"
				continue
			}
			if status == "created" {
				if err := os.Rename(filepath.Join(dir, old), target); err != nil {
					return nil, err
				}
				res.Renamed = append(res.Renamed, exportRename{d.ID, old, d.File})
				status = "renamed"
			} else {
				x.e.warnf("#%d has another file in %s: %s (left alone)", d.ID, dir, old)
			}
		}
		if cur, err := os.ReadFile(target); err == nil && bytes.Equal(cur, d.Content) {
			if status == "updated" {
				status = "unchanged"
			}
		} else if err := os.WriteFile(target, d.Content, 0o644); err != nil {
			return nil, err
		}
		switch status {
		case "created":
			res.Created++
		case "updated":
			res.Updated++
		case "unchanged":
			res.Unchanged++
		}
		res.Files = append(res.Files, exportFile{d.ID, d.File, status})
		m.WorkPackages = append(m.WorkPackages, manifestEntry{d.ID, d.File, d.UpdatedAt})
	}
	for id := range prev {
		if !seen[id] {
			res.Gone = append(res.Gone, id)
		}
	}
	sort.Ints(res.Gone)
	sort.Slice(m.WorkPackages, func(i, j int) bool { return m.WorkPackages[i].ID < m.WorkPackages[j].ID })
	if m.WorkPackages == nil {
		m.WorkPackages = []manifestEntry{}
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, exportManifest), append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return res, nil
}

// existingFiles lists the files of work package id in dir: <id>.md and
// <id>-*.md.
func existingFiles(dir string, id int) []string {
	var out []string
	prefix := strconv.Itoa(id)
	entries, _ := os.ReadDir(dir)
	for _, en := range entries {
		n := en.Name()
		if en.IsDir() || !strings.HasSuffix(n, ".md") {
			continue
		}
		if n == prefix+".md" || strings.HasPrefix(n, prefix+"-") {
			out = append(out, n)
		}
	}
	return out
}

// downloadAttachments copies into DIR/attachments/ the attachments the
// texts link to (and, with --attachments all, every attachment of each work
// package), so links can point at local files. A file already there with
// the attachment's size is not downloaded again; one that fails stays a
// server link, with a warning.
func (x *exporter) downloadAttachments(ctx context.Context, dir string, items []map[string]any, comments map[int][]exportComment) (int, error) {
	x.local, x.listed = map[int]string{}, map[int][]exportAttachment{}
	want := map[int]bool{}
	re := x.attachmentRe()
	for _, it := range items {
		texts := []string{fmt.Sprint(firstNonNil(it["description"], ""))}
		for _, c := range comments[intOf(it["id"])] {
			texts = append(texts, c.Text)
		}
		for _, t := range texts {
			for _, m := range re.FindAllStringSubmatch(t, -1) {
				id, _ := strconv.Atoi(m[1])
				want[id] = true
			}
		}
	}
	all := x.a.String("attachments") == "all"
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		sem      = make(chan struct{}, 8)
		metas    = map[int]map[string]any{}
	)
	if all {
		for _, it := range items {
			wpID := intOf(it["id"])
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				atts, _, err := x.e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/attachments", wpID), nil, 0)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("attachments of #%d: %w", wpID, err)
					}
					return
				}
				for _, at := range atts {
					id := intOf(at["id"])
					want[id], metas[id] = true, at
					x.listed[wpID] = append(x.listed[wpID], exportAttachment{id, fmt.Sprint(at["fileName"])})
				}
				sort.Slice(x.listed[wpID], func(i, j int) bool { return x.listed[wpID][i].ID < x.listed[wpID][j].ID })
			}()
		}
		wg.Wait()
		if firstErr != nil {
			return 0, firstErr
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Join(dir, "attachments"), 0o755); err != nil {
		return 0, err
	}
	downloaded := 0
	for id := range want {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			mu.Lock()
			meta := metas[id]
			mu.Unlock()
			rel, fresh, err := x.downloadAttachment(ctx, dir, id, meta)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				x.e.warnf("attachment %d not downloaded, left as a server link: %v", id, err)
				return
			}
			x.local[id] = rel
			if fresh {
				downloaded++
			}
		}()
	}
	wg.Wait()
	return downloaded, nil
}

func (x *exporter) downloadAttachment(ctx context.Context, dir string, id int, meta map[string]any) (string, bool, error) {
	if meta == nil {
		m, err := x.e.C.Get(ctx, fmt.Sprintf("/attachments/%d", id), nil)
		if err != nil {
			return "", false, err
		}
		meta = m
	}
	rel := "attachments/" + attachmentFileName(id, fmt.Sprint(meta["fileName"]))
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if st, err := os.Stat(path); err == nil && st.Size() == int64(intOf(meta["fileSize"])) {
		return rel, false, nil
	}
	loc := hal.Link(meta, "downloadLocation")
	if loc == "" {
		return "", false, fmt.Errorf("no download location")
	}
	data, err := x.e.C.Download(ctx, loc)
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", false, err
	}
	return rel, true, nil
}

// attachmentFileName is <id>-<name>, with whatever would break a Markdown
// link or a file system turned into dashes.
func attachmentFileName(id int, name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return fmt.Sprintf("%d-%s", id, strings.Trim(b.String(), "-"))
}
