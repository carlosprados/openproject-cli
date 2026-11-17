package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Resolution of human references (names, identifiers, "me") into API ids.
// Every resolver accepts a numeric id as-is, so callers that already know
// ids never pay an extra request. Catalog lookups are cached per process.

type resolverCache struct {
	mu       sync.Mutex
	catalogs map[string][]named
	projects map[string]int
	// workPackages maps a project-based work package identifier ("PROJ-42")
	// to its numeric id, so each one costs a single request per process.
	workPackages map[string]int
	me           int
	schemas      map[string]map[string]string // "project-type" → customFieldN → name
	loc          *time.Location
	statuses     map[int]statusInfo
	// statusLabel is the localised name of the status field, as activity
	// details print it ("Status", "Estado").
	statusLabel string
	warnMu      sync.Mutex
}

type named struct {
	ID   int
	Name string
}

func (e *Env) catalog(ctx context.Context, path string) ([]named, error) {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()
	if e.cache.catalogs == nil {
		e.cache.catalogs = map[string][]named{}
	}
	if c, ok := e.cache.catalogs[path]; ok {
		return c, nil
	}
	items, _, err := e.C.Collect(ctx, path, nil, 0)
	if err != nil {
		return nil, err
	}
	out := make([]named, 0, len(items))
	for _, it := range items {
		out = append(out, named{ID: intOf(it["id"]), Name: fmt.Sprint(it["name"])})
	}
	e.cache.catalogs[path] = out
	return out, nil
}

func pick(kind, ref string, items []named) (int, error) {
	if id, err := strconv.Atoi(strings.TrimPrefix(ref, "#")); err == nil {
		return id, nil
	}
	var partial []named
	for _, it := range items {
		if strings.EqualFold(it.Name, ref) {
			return it.ID, nil
		}
		if strings.Contains(strings.ToLower(it.Name), strings.ToLower(ref)) {
			partial = append(partial, it)
		}
	}
	if len(partial) == 1 {
		return partial[0].ID, nil
	}
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = fmt.Sprintf("%s (%d)", it.Name, it.ID)
	}
	if len(partial) > 1 {
		return 0, fmt.Errorf("%s %q is ambiguous; available: %s", kind, ref, strings.Join(names, ", "))
	}
	return 0, fmt.Errorf("unknown %s %q; available: %s", kind, ref, strings.Join(names, ", "))
}

// StatusID resolves a status name or id.
func (e *Env) StatusID(ctx context.Context, ref string) (int, error) {
	items, err := e.catalog(ctx, "/statuses")
	if err != nil {
		return 0, err
	}
	return pick("status", ref, items)
}

// TypeID resolves a work package type name or id.
func (e *Env) TypeID(ctx context.Context, ref string) (int, error) {
	items, err := e.catalog(ctx, "/types")
	if err != nil {
		return 0, err
	}
	return pick("type", ref, items)
}

// defaultTypeID picks the type of a new work package: the configured
// default_type, else the type the project marks as default, else its first.
func (e *Env) defaultTypeID(ctx context.Context, projectID int) (int, error) {
	if e.DefaultType != "" {
		return e.TypeID(ctx, e.DefaultType)
	}
	res, err := e.C.Get(ctx, fmt.Sprintf("/projects/%d/types", projectID), nil)
	if err != nil {
		return 0, err
	}
	els := client.Elements(res)
	if len(els) == 0 {
		return 0, fmt.Errorf("project %d has no work package types enabled", projectID)
	}
	for _, t := range els {
		if t["isDefault"] == true {
			return intOf(t["id"]), nil
		}
	}
	return intOf(els[0]["id"]), nil
}

// PriorityID resolves a priority name or id.
func (e *Env) PriorityID(ctx context.Context, ref string) (int, error) {
	items, err := e.catalog(ctx, "/priorities")
	if err != nil {
		return 0, err
	}
	return pick("priority", ref, items)
}

// RoleID resolves a role name or id.
func (e *Env) RoleID(ctx context.Context, ref string) (int, error) {
	items, err := e.catalog(ctx, "/roles")
	if err != nil {
		return 0, err
	}
	return pick("role", ref, items)
}

// VersionID resolves a version name or id within a project.
func (e *Env) VersionID(ctx context.Context, projectID int, ref string) (int, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		return id, nil
	}
	if projectID == 0 {
		return 0, fmt.Errorf("version %q given by name: a project is required to resolve it", ref)
	}
	items, err := e.catalog(ctx, fmt.Sprintf("/projects/%d/versions", projectID))
	if err != nil {
		return 0, err
	}
	return pick("version", ref, items)
}

// CategoryID resolves a category name or id within a project.
func (e *Env) CategoryID(ctx context.Context, projectID int, ref string) (int, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		return id, nil
	}
	items, err := e.catalog(ctx, fmt.Sprintf("/projects/%d/categories", projectID))
	if err != nil {
		return 0, err
	}
	return pick("category", ref, items)
}

// ActivityID resolves a time-entry activity name or id. Activities are only
// listed through the time entry form, so the work package is needed.
func (e *Env) ActivityID(ctx context.Context, wpID int, ref string) (int, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		return id, nil
	}
	items, err := e.Activities(ctx, wpID)
	if err != nil {
		return 0, err
	}
	return pick("activity", ref, items)
}

// Activities lists the time-entry activities allowed for a work package.
func (e *Env) Activities(ctx context.Context, wpID int) ([]named, error) {
	links := map[string]any{}
	if wpID > 0 {
		links["entity"] = hal.Ref(href("work_packages", wpID))
	}
	return e.activitiesForm(ctx, links)
}

// ProjectActivities lists the time-entry activities enabled in a project,
// for when the work package does not exist yet (the form returns none
// without an entity or project).
func (e *Env) ProjectActivities(ctx context.Context, projectID int) ([]named, error) {
	return e.activitiesForm(ctx, map[string]any{"project": hal.Ref(href("projects", projectID))})
}

func (e *Env) activitiesForm(ctx context.Context, links map[string]any) ([]named, error) {
	body := map[string]any{}
	if len(links) > 0 {
		body["_links"] = links
	}
	form, err := e.C.Post(ctx, "/time_entries/form", body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Embedded struct {
			Schema struct {
				Activity struct {
					Embedded struct {
						AllowedValues []struct {
							ID   int    `json:"id"`
							Name string `json:"name"`
						} `json:"allowedValues"`
					} `json:"_embedded"`
				} `json:"activity"`
			} `json:"schema"`
		} `json:"_embedded"`
	}
	if err := remarshal(form, &parsed); err != nil {
		return nil, err
	}
	var out []named
	for _, v := range parsed.Embedded.Schema.Activity.Embedded.AllowedValues {
		out = append(out, named{ID: v.ID, Name: v.Name})
	}
	return out, nil
}

// MeID returns the id of the authenticated user.
func (e *Env) MeID(ctx context.Context) (int, error) {
	e.cache.mu.Lock()
	me := e.cache.me
	e.cache.mu.Unlock()
	if me != 0 {
		return me, nil
	}
	res, err := e.C.Get(ctx, "/users/me", nil)
	if err != nil {
		return 0, err
	}
	me = intOf(res["id"])
	e.cache.mu.Lock()
	e.cache.me = me
	e.cache.mu.Unlock()
	return me, nil
}

// UserID resolves "me", an id, a login, an email or a (partial) name to a
// principal id (users, groups and placeholder users).
func (e *Env) UserID(ctx context.Context, ref string) (int, error) {
	if strings.EqualFold(ref, "me") {
		return e.MeID(ctx)
	}
	if id, err := strconv.Atoi(ref); err == nil {
		return id, nil
	}
	f := NewFilters().Add("any_name_attribute", "~", ref)
	items, _, err := e.C.Collect(ctx, "/principals", url.Values{"filters": {f.JSON()}}, 50)
	if err != nil {
		return 0, err
	}
	var candidates []string
	for _, it := range items {
		for _, k := range []string{"login", "email", "name"} {
			if s, _ := it[k].(string); strings.EqualFold(s, ref) {
				return intOf(it["id"]), nil
			}
		}
		candidates = append(candidates, fmt.Sprintf("%v (%v)", it["name"], it["id"]))
	}
	switch len(items) {
	case 0:
		return 0, fmt.Errorf("no user matches %q", ref)
	case 1:
		return intOf(items[0]["id"]), nil
	}
	return 0, fmt.Errorf("user %q is ambiguous: %s", ref, strings.Join(candidates, ", "))
}

// UserHref resolves a user reference into a link href; "none" or an empty
// string unset the link.
func (e *Env) UserHref(ctx context.Context, ref string) (string, error) {
	if ref == "" || strings.EqualFold(ref, "none") {
		return "", nil
	}
	id, err := e.UserID(ctx, ref)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/api/v3/users/%d", id), nil
}

// ProjectID resolves an id, identifier or (partial) name. Empty ref falls
// back to the configured default project.
func (e *Env) ProjectID(ctx context.Context, ref string) (int, error) {
	if ref == "" {
		ref = e.DefaultProject
	}
	if ref == "" {
		return 0, fmt.Errorf("no project given and no default project configured (use --project or set `project` in the config)")
	}
	if id, err := strconv.Atoi(ref); err == nil {
		return id, nil
	}
	e.cache.mu.Lock()
	if id, ok := e.cache.projects[ref]; ok {
		e.cache.mu.Unlock()
		return id, nil
	}
	e.cache.mu.Unlock()

	id := 0
	res, err := e.C.Get(ctx, "/projects/"+url.PathEscape(ref), nil)
	switch {
	case err == nil:
		id = intOf(res["id"])
	case client.IsNotFound(err):
		f := NewFilters().Add("name_and_identifier", "~", ref)
		items, _, err := e.C.Collect(ctx, "/projects", url.Values{"filters": {f.JSON()}}, 20)
		if err != nil {
			return 0, err
		}
		list := make([]named, len(items))
		for i, it := range items {
			list[i] = named{ID: intOf(it["id"]), Name: fmt.Sprint(it["name"])}
		}
		if len(list) == 0 {
			return 0, fmt.Errorf("no project matches %q (try `opcli project list`)", ref)
		}
		if id, err = pick("project", ref, list); err != nil {
			return 0, err
		}
	default:
		return 0, err
	}
	e.cache.mu.Lock()
	if e.cache.projects == nil {
		e.cache.projects = map[string]int{}
	}
	e.cache.projects[ref] = id
	e.cache.mu.Unlock()
	return id, nil
}

// workPackageScope adds pid to the work package filters f and returns the
// collection to read them from. Without subprojects it is a "project ="
// filter on /work_packages, which follows the instance setting "display
// subprojects work packages". With subprojects it reads
// /projects/{pid}/work_packages with a "subprojectId =" filter listing every
// project below pid, so the result does not depend on that setting. The
// listed projects are those whose parent links reach pid (see projectTree):
// a "project =" filter expands each id through the server's nested set, which
// can hold projects that are not in the tree the web UI shows.
func (e *Env) workPackageScope(ctx context.Context, f Filters, pid int, subprojects bool) (Filters, string, error) {
	if !subprojects {
		return f.Add("project", "=", pid), "/work_packages", nil
	}
	anc := NewFilters().Add("ancestor", "=", pid)
	items, _, err := e.C.Collect(ctx, "/projects", url.Values{"filters": {anc.JSON()}}, 0)
	if err != nil {
		return f, "", err
	}
	paths, skipped := projectTree(pid, append([]map[string]any{{"id": pid}}, hal.FlattenAll(items)...))
	if len(skipped) > 0 {
		e.warnf("left out %s: the server lists them below project %d but their parent chain does not reach it", strings.Join(skipped, ", "), pid)
	}
	var ids []int
	for id := range paths {
		if id != pid {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	path := fmt.Sprintf("/projects/%d/work_packages", pid)
	if len(ids) == 0 {
		return f.Add("subprojectId", "!*"), path, nil
	}
	vals := make([]any, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	return f.Add("subprojectId", "=", vals...), path, nil
}

// projectTree follows the parent links of projects (the root and those the
// ancestor filter returned) and gives the path of identifiers from the root
// of each one that reaches it. The ancestor filter follows the server's
// nested set, which can disagree with the parent links; the parent links are
// what the web UI shows, so they decide. The rest come back as skipped
// ("IDENT (#id)"), sorted.
func projectTree(rootID int, projects []map[string]any) (map[int][]string, []string) {
	byID := map[int]map[string]any{}
	for _, p := range projects {
		byID[intOf(p["id"])] = p
	}
	paths := map[int][]string{}
	var skipped []string
	for id, p := range byID {
		var rev []string
		reached := false
		for cur, hops := id, 0; hops <= len(byID) && !reached; hops++ {
			q := byID[cur]
			if q == nil {
				break
			}
			rev = append(rev, fmt.Sprint(q["identifier"]))
			reached = cur == rootID
			cur = intOf(q["parentId"])
		}
		if !reached {
			skipped = append(skipped, fmt.Sprintf("%v (#%d)", p["identifier"], id))
			continue
		}
		for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
			rev[i], rev[j] = rev[j], rev[i]
		}
		paths[id] = rev
	}
	sort.Strings(skipped)
	return paths, skipped
}

// workPackageDisplayID matches the project-based identifier the web UI shows for
// a work package (its displayId), such as "PROJ-42": a project prefix, a hyphen
// and the work package's position in that project. The prefix may carry hyphens
// itself, so only the last "-<digits>" separates it.
var workPackageDisplayID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*-[0-9]+$`)

// WorkPackage resolves a work package reference to its numeric id. It takes
// what people, branch names and commit messages carry — "123", "#123" or the
// project-based identifier the web UI shows ("PROJ-42") — because every other
// call needs the numeric id. A numeric reference costs no request; a
// project-based one costs one GET, cached for the rest of the process.
//
// API v3 accepts the project-based identifier in place of the id in
// /work_packages/{id} on instances that have them, and matches its prefix in
// upper case only ("proj-42" is not found), so the prefix is upper-cased first.
// On instances without project-based identifiers the GET is a 404, and the
// error says the numeric id is the way.
func (e *Env) WorkPackage(ctx context.Context, ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	if id, err := WorkPackageID(ref); err == nil {
		return id, nil
	}
	if !workPackageDisplayID.MatchString(ref) {
		return 0, fmt.Errorf("invalid work package id %q: use 123, #123 or a project-based id such as PROJ-123", ref)
	}
	key := strings.ToUpper(ref)
	e.cache.mu.Lock()
	id, ok := e.cache.workPackages[key]
	e.cache.mu.Unlock()
	if ok {
		return id, nil
	}
	res, err := e.C.Get(ctx, "/work_packages/"+url.PathEscape(key), nil)
	if err != nil {
		if client.IsNotFound(err) {
			return 0, fmt.Errorf("work package %s not found (project-based ids need an OpenProject that has them; the numeric id always works)", key)
		}
		return 0, err
	}
	if id = intOf(res["id"]); id <= 0 {
		return 0, fmt.Errorf("work package %s: the server answered without an id", key)
	}
	e.cache.mu.Lock()
	if e.cache.workPackages == nil {
		e.cache.workPackages = map[string]int{}
	}
	e.cache.workPackages[key] = id
	e.cache.mu.Unlock()
	return id, nil
}

// workPackages resolves a comma- or space-separated list of work package
// references (see WorkPackage), for the commands that act on several at once.
func (e *Env) workPackages(ctx context.Context, refs string) ([]int, error) {
	var ids []int
	for _, f := range strings.FieldsFunc(refs, func(r rune) bool { return r == ',' || r == ' ' }) {
		id, err := e.WorkPackage(ctx, f)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("missing work package id")
	}
	return ids, nil
}

// WorkPackageID parses a numeric reference, "123" or "#123", with no request.
// Work package commands go through Env.WorkPackage, which also takes
// project-based identifiers; this one stays for ids that are not work packages
// (meetings, agenda items) and for callers that must not hit the server.
func WorkPackageID(ref string) (int, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(ref), "#"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid work package id %q", ref)
	}
	return id, nil
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func remarshal(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// Filters builds the JSON "filters" query parameter of API v3 collections.
type Filters []map[string]any

// NewFilters starts an empty filter list.
func NewFilters() Filters { return Filters{} }

// Add appends a filter; values are stringified.
func (f Filters) Add(name, operator string, values ...any) Filters {
	vals := make([]string, 0, len(values))
	for _, v := range values {
		vals = append(vals, fmt.Sprint(v))
	}
	return append(f, map[string]any{name: map[string]any{"operator": operator, "values": vals}})
}

// JSON serialises the filters.
func (f Filters) JSON() string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep operators like "<t+" readable
	_ = enc.Encode(f)
	return strings.TrimSpace(buf.String())
}

// Merge appends raw user-provided filters (a JSON array in API format).
func (f Filters) Merge(raw string) (Filters, error) {
	if strings.TrimSpace(raw) == "" {
		return f, nil
	}
	var extra []map[string]any
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return nil, fmt.Errorf(`invalid filter JSON (expected e.g. [{"status":{"operator":"o","values":[]}}]): %w`, err)
	}
	return append(f, extra...), nil
}
