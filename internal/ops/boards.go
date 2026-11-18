package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// A board is a grid scoped to /projects/<identifier>/boards. Each column is
// a work_package_query widget pointing to a hidden query of its own: in an
// action board the query filters by the board's attribute (one value per
// column); in a free board it holds the cards it was given (manualSort).
// The order of the cards inside a column is the query's manual order
// (/queries/{id}/order: work package id → position), then by id.

func init() {
	registerGroup(&Group{
		Name:    "board",
		Aliases: []string{"boards"},
		Short:   "Boards (kanban): columns, cards and their order",
		Long: `A board shows work packages in columns. In a board by attribute (status,
assignee, version, subproject, subtasks) each column is one value of that
field, and moving a card between columns changes the field. In a free board
the columns are lists you fill by hand. Inside a column, cards keep a manual
order.

Boards are referenced by id or name (with --project when the name is not
unique); columns by name or by position (1 = leftmost).`,
	})
	registerGroup(&Group{Name: "board column", Aliases: []string{"columns", "col"}, Short: "Columns of a board: add, remove, move, rename"})
	registerGroup(&Group{Name: "board card", Aliases: []string{"cards"}, Short: "Cards on a board: move between columns, reorder, add to or remove from free lists"})

	register(boardList, boardGet, boardCreate, boardUpdate, boardDelete,
		boardColumnAdd, boardColumnRemove, boardColumnMove, boardColumnRename,
		boardCardMove, boardCardAdd, boardCardRemove)
}

// boardAttr describes one kind of action board.
type boardAttr struct {
	filter  string   // query filter and widget filter key the web writes
	keys    []string // widget filter keys that may carry the value
	wpArg   string   // wp update param that changes it
	kind    string   // API resource of the values
	wpLinks []string // work package links that hold the value
}

var boardAttrs = map[string]boardAttr{
	"status":     {filter: "status", keys: []string{"status", "status_id"}, wpArg: "status", kind: "statuses", wpLinks: []string{"status"}},
	"assignee":   {filter: "assignee", keys: []string{"assignee", "assigned_to_id"}, wpArg: "assignee", kind: "users", wpLinks: []string{"assignee"}},
	"version":    {filter: "version", keys: []string{"version", "fixed_version_id", "targetVersion", "targetVersions"}, wpArg: "version", kind: "versions", wpLinks: []string{"version", "targetVersions"}},
	"subproject": {filter: "onlySubproject", keys: []string{"onlySubproject", "only_subproject_id"}, wpArg: "project", kind: "projects", wpLinks: []string{"project"}},
	"subtasks":   {filter: "parent", keys: []string{"parent", "parent_id"}, wpArg: "parent", kind: "work_packages", wpLinks: []string{"parent"}},
}

var boardKinds = []string{"status", "assignee", "version", "subproject", "subtasks", "free"}

// orderDistance is the gap the web leaves between card positions.
const orderDistance = 16384

type board struct {
	ID        int
	Name      string
	Scope     string // /projects/<identifier>/boards
	Project   string // identifier
	Kind      string // action | free
	Attribute string
	Filters   []any // board-wide filters, on top of each column's
	Columns   []*boardCol
	CreatedAt string
	UpdatedAt string
}

type boardCol struct {
	WidgetID int
	QueryID  int
	Filters  []any
	Options  map[string]any
	Value    string // the attribute value's id in an action board
	Name     string
	SubProj  bool // the query includes subprojects
}

func (b *board) attr() (boardAttr, bool) {
	a, ok := boardAttrs[b.Attribute]
	return a, ok && b.Kind == "action"
}

func parseBoard(res map[string]any) (*board, error) {
	scope := hal.Link(res, "scope")
	if !strings.HasPrefix(scope, "/projects/") || !strings.HasSuffix(scope, "/boards") {
		return nil, fmt.Errorf("grid %v is not a board (scope %q)", res["id"], scope)
	}
	opts, _ := res["options"].(map[string]any)
	b := &board{
		ID: intOf(res["id"]), Name: hal.String(res["name"]), Scope: scope,
		Project:   strings.TrimSuffix(strings.TrimPrefix(scope, "/projects/"), "/boards"),
		Kind:      "free",
		CreatedAt: hal.String(res["createdAt"]), UpdatedAt: hal.String(res["updatedAt"]),
	}
	if hal.String(opts["type"]) == "action" {
		b.Kind, b.Attribute = "action", hal.String(opts["attribute"])
	}
	b.Filters, _ = opts["filters"].([]any)
	type placed struct {
		col   *boardCol
		start int
	}
	var cols []placed
	ws, _ := res["widgets"].([]any)
	for _, w := range ws {
		wm, _ := w.(map[string]any)
		if hal.String(wm["identifier"]) != "work_package_query" {
			continue
		}
		o, _ := wm["options"].(map[string]any)
		c := &boardCol{WidgetID: intOf(wm["id"]), QueryID: intOf(o["queryId"]), Options: o, SubProj: true}
		c.Filters, _ = o["filters"].([]any)
		if a, ok := b.attr(); ok {
			c.Value = filterValue(c.Filters, a.keys)
		}
		cols = append(cols, placed{c, intOf(wm["startColumn"])})
	}
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].start < cols[j].start })
	for _, p := range cols {
		b.Columns = append(b.Columns, p.col)
	}
	return b, nil
}

// filterValue returns the first value of the first filter named by one of
// keys, in API filter JSON ([{"status":{"operator":"=","values":["7"]}}]).
func filterValue(filters []any, keys []string) string {
	for _, f := range filters {
		fm, _ := f.(map[string]any)
		for _, k := range keys {
			if spec, ok := fm[k].(map[string]any); ok {
				if vals, _ := spec["values"].([]any); len(vals) > 0 {
					return hal.String(vals[0])
				}
			}
		}
	}
	return ""
}

func (e *Env) boardsOf(ctx context.Context, project string) ([]*board, error) {
	q := url.Values{}
	if !strings.EqualFold(project, "all") && (project != "" || e.DefaultProject != "") {
		pid, err := e.ProjectID(ctx, project)
		if err != nil {
			return nil, err
		}
		q.Set("filters", fmt.Sprintf(`[{"scope":{"operator":"=","values":["/projects/%d/boards"]}}]`, pid))
	}
	items, _, err := e.C.Collect(ctx, "/grids", q, 0)
	if err != nil {
		return nil, err
	}
	var out []*board
	for _, it := range items {
		if b, err := parseBoard(it); err == nil {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// findBoard resolves a board by id, or by name among the boards of project
// (every visible board when there is no project).
func (e *Env) findBoard(ctx context.Context, ref, project string) (*board, error) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "#")
	if ref == "" {
		return nil, fmt.Errorf("which board? pass its id or name (opcli board list)")
	}
	if id, err := strconv.Atoi(ref); err == nil {
		res, err := e.C.Get(ctx, fmt.Sprintf("/grids/%d", id), nil)
		if err != nil {
			return nil, err
		}
		return parseBoard(res)
	}
	boards, err := e.boardsOf(ctx, project)
	if err != nil {
		return nil, err
	}
	var exact, partial []*board
	for _, b := range boards {
		switch {
		case strings.EqualFold(b.Name, ref):
			exact = append(exact, b)
		case strings.Contains(strings.ToLower(b.Name), strings.ToLower(ref)):
			partial = append(partial, b)
		}
	}
	if len(exact) == 0 {
		exact = partial
	}
	switch len(exact) {
	case 1:
		// The listing has every field, but re-read it: widgets may be trimmed.
		res, err := e.C.Get(ctx, fmt.Sprintf("/grids/%d", exact[0].ID), nil)
		if err != nil {
			return nil, err
		}
		return parseBoard(res)
	case 0:
		var names []string
		for _, b := range boards {
			names = append(names, fmt.Sprintf("%d %q (%s)", b.ID, b.Name, b.Project))
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no board %q: there are no boards here", ref)
		}
		return nil, fmt.Errorf("no board %q; boards: %s", ref, strings.Join(names, ", "))
	}
	var names []string
	for _, b := range exact {
		names = append(names, fmt.Sprintf("%d %q (%s)", b.ID, b.Name, b.Project))
	}
	return nil, fmt.Errorf("board %q is ambiguous: %s; pass the id or --project", ref, strings.Join(names, ", "))
}

// loadColumns fills the column names and subproject flags from their queries.
func (e *Env) loadColumns(ctx context.Context, b *board) error {
	if len(b.Columns) == 0 {
		return nil
	}
	ids := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		ids[i] = strconv.Itoa(c.QueryID)
	}
	q := url.Values{}
	q.Set("filters", NewFilters().Add("id", "=", toAny(ids)...).JSON())
	items, _, err := e.C.Collect(ctx, "/queries", q, 0)
	if err != nil {
		return err
	}
	byID := map[int]map[string]any{}
	for _, it := range items {
		byID[intOf(it["id"])] = it
	}
	for i, c := range b.Columns {
		if qr, ok := byID[c.QueryID]; ok {
			c.Name = hal.String(qr["name"])
			c.SubProj = qr["includeSubprojects"] != false
		} else {
			c.Name = fmt.Sprintf("column %d", i+1)
		}
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// findColumn resolves a column by name (case-insensitive) or position.
func (b *board) findColumn(ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	for i, c := range b.Columns {
		if strings.EqualFold(c.Name, ref) {
			return i, nil
		}
	}
	if n, err := strconv.Atoi(ref); err == nil && n >= 1 && n <= len(b.Columns) {
		return n - 1, nil
	}
	var names []string
	for i, c := range b.Columns {
		names = append(names, fmt.Sprintf("%d %q", i+1, c.Name))
	}
	if len(names) == 0 {
		return 0, fmt.Errorf("board %q has no columns", b.Name)
	}
	return 0, fmt.Errorf("no column %q in board %q; columns: %s", ref, b.Name, strings.Join(names, ", "))
}

// queryOrder reads the manual order of a query: work package id → position.
func (e *Env) queryOrder(ctx context.Context, qid int) (map[int]int, error) {
	if qid <= 0 {
		return map[int]int{}, nil
	}
	res, err := e.C.Get(ctx, fmt.Sprintf("/queries/%d/order", qid), nil)
	if err != nil {
		return nil, err
	}
	out := make(map[int]int, len(res))
	for k, v := range res {
		id, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		if f, ok := v.(float64); ok {
			out[id] = int(f)
		}
	}
	return out, nil
}

// columnCards returns the cards of a column in board order and their
// positions. max > 0 only caps what is returned; the order needs them all.
func (e *Env) columnCards(ctx context.Context, b *board, pid int, c *boardCol) ([]map[string]any, map[int]int, int, error) {
	pos, err := e.queryOrder(ctx, c.QueryID)
	if err != nil {
		return nil, nil, 0, err
	}
	filters := append([]any{}, b.Filters...)
	q := url.Values{}
	path := fmt.Sprintf("/projects/%d/work_packages", pid)
	if b.Kind == "free" {
		if len(pos) == 0 {
			return []map[string]any{}, pos, 0, nil
		}
		var ids []string
		for id := range pos {
			ids = append(ids, strconv.Itoa(id))
		}
		sort.Strings(ids)
		filters = append(filters, map[string]any{"id": map[string]any{"operator": "=", "values": ids}})
		path = "/work_packages"
	} else {
		filters = append(filters, c.Filters...)
		q.Set("includeSubprojects", strconv.FormatBool(c.SubProj))
	}
	fj, _ := json.Marshal(filters)
	q.Set("filters", string(fj))
	q.Set("sortBy", `[["id","asc"]]`)
	items, total, err := e.C.CollectWith(ctx, path, q, 0, e.wpPageFallback(path))
	if err != nil {
		return nil, nil, 0, err
	}
	sortCards(items, pos)
	return items, pos, total, nil
}

// sortCards orders work packages as a manually sorted query does: by
// position, those without one last, then by id.
func sortCards(items []map[string]any, pos map[int]int) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := intOf(items[i]["id"]), intOf(items[j]["id"])
		pa, oka := pos[a]
		pb, okb := pos[b]
		switch {
		case oka && okb && pa != pb:
			return pa < pb
		case oka != okb:
			return oka
		}
		return a < b
	})
}

// placeCard returns the order delta (work package id → position) that puts
// id at index at of order, the column's cards in board order without id.
// It writes one position when there is room between the neighbours and
// renumbers the head of the column otherwise.
func placeCard(order []int, pos map[int]int, id, at int) map[int]int {
	delta := placeDelta(order, pos, id, at)
	// The order API reads -1 as "remove from the order".
	for w, p := range delta {
		if p == -1 {
			delta[w] = -2
		}
	}
	return delta
}

func placeDelta(order []int, pos map[int]int, id, at int) map[int]int {
	at = max(0, min(at, len(order)))
	const lo, hi = -2147483647, 2147483647
	next, hasNext := 0, false
	if at < len(order) {
		next, hasNext = pos[order[at]]
	}
	if at == 0 {
		switch {
		case !hasNext:
			// Every card is unpositioned: any position puts id first.
			return map[int]int{id: 0}
		case next-orderDistance > lo:
			return map[int]int{id: next - orderDistance}
		}
	} else if prev, ok := pos[order[at-1]]; ok {
		switch {
		case !hasNext && prev+orderDistance < hi:
			// Only unpositioned cards follow, and they sort after any position.
			return map[int]int{id: prev + orderDistance}
		case hasNext && next-prev >= 2:
			return map[int]int{id: prev + (next-prev)/2}
		}
	}
	list := make([]int, 0, len(order)+1)
	list = append(list, order[:at]...)
	list = append(list, id)
	list = append(list, order[at:]...)
	last := at
	for i, w := range list {
		if _, ok := pos[w]; ok && w != id {
			last = max(last, i)
		}
	}
	delta := map[int]int{}
	for i := 0; i <= last; i++ {
		p := i * orderDistance
		if cur, ok := pos[list[i]]; !ok || cur != p || list[i] == id {
			delta[list[i]] = p
		}
	}
	return delta
}

func (e *Env) patchOrder(ctx context.Context, qid int, delta map[int]int) error {
	if len(delta) == 0 {
		return nil
	}
	d := make(map[string]int, len(delta))
	for id, p := range delta {
		d[strconv.Itoa(id)] = p
	}
	_, err := e.C.Patch(ctx, fmt.Sprintf("/queries/%d/order", qid), map[string]any{"delta": d})
	return err
}

// --- output --------------------------------------------------------------

type boardView struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Project   string       `json:"project"`
	Kind      string       `json:"kind"`
	Attribute string       `json:"attribute,omitempty"`
	Filters   []any        `json:"filters,omitempty"`
	URL       string       `json:"url"`
	Columns   []columnView `json:"columns"`
	cards     bool
}

type columnView struct {
	Position int              `json:"position"`
	Name     string           `json:"name"`
	Value    string           `json:"value,omitempty"`
	QueryID  int              `json:"queryId"`
	WidgetID int              `json:"widgetId"`
	Total    *int             `json:"total,omitempty"`
	Cards    []map[string]any `json:"cards,omitempty"`
}

var cardFields = []string{"id", "displayId", "subject", "type", "status", "assignee", "priority", "dueDate", "project"}

func (e *Env) boardURL(b *board) string { return e.C.WebURL(fmt.Sprintf("%s/%d", b.Scope, b.ID)) }

func (e *Env) viewBoard(b *board) *boardView {
	v := &boardView{ID: b.ID, Name: b.Name, Project: b.Project, Kind: b.Kind, Filters: b.Filters, URL: e.boardURL(b), Columns: []columnView{}}
	if b.Kind == "action" {
		v.Attribute = b.Attribute
	}
	for i, c := range b.Columns {
		v.Columns = append(v.Columns, columnView{Position: i + 1, Name: c.Name, Value: c.Value, QueryID: c.QueryID, WidgetID: c.WidgetID})
	}
	return v
}

func card(wp map[string]any, pos map[int]int, project string) map[string]any {
	f := hal.Flatten(wp)
	out := map[string]any{}
	for _, k := range cardFields {
		if v, ok := f[k]; ok && v != nil && v != "" {
			out[k] = v
		}
	}
	if d, ok := out["displayId"]; ok && fmt.Sprint(d) == fmt.Sprint(out["id"]) {
		delete(out, "displayId")
	}
	// Only cards from another project (subprojects) carry it.
	if pi, ok := f["projectId"]; ok && fmt.Sprint(pi) == project {
		delete(out, "project")
	}
	if p, ok := pos[intOf(out["id"])]; ok {
		out["position"] = p
	}
	return out
}

func renderBoard(w io.Writer, v any) error {
	b, ok := v.(*boardView)
	if !ok {
		return RenderJSON(w, v)
	}
	kind := "free board"
	if b.Kind == "action" {
		kind = "board by " + b.Attribute
	}
	fmt.Fprintf(w, "#%d %s (%s, %s)\n%s\n", b.ID, b.Name, kind, b.Project, b.URL)
	for _, c := range b.Columns {
		total := ""
		if c.Total != nil {
			total = fmt.Sprintf(" (%d)", *c.Total)
		}
		fmt.Fprintf(w, "\n%d. %s%s\n", c.Position, c.Name, total)
		for _, cd := range c.Cards {
			ref := "#" + fmt.Sprint(cd["id"])
			if d, ok := cd["displayId"]; ok {
				ref += " " + fmt.Sprint(d)
			}
			extra := []string{}
			for _, k := range []string{"status", "assignee"} {
				if s := hal.String(cd[k]); s != "" && !(b.Attribute == k) {
					extra = append(extra, s)
				}
			}
			suffix := ""
			if len(extra) > 0 {
				suffix = "  [" + strings.Join(extra, ", ") + "]"
			}
			fmt.Fprintf(w, "   %-16s %s%s\n", ref, hal.String(cd["subject"]), suffix)
		}
		if c.Total != nil && len(c.Cards) < *c.Total && b.cards {
			fmt.Fprintf(w, "   … %d more\n", *c.Total-len(c.Cards))
		}
	}
	return nil
}

// --- read ops ------------------------------------------------------------

var pBoard = Param{Name: "board", Kind: String, Required: true, Positional: true, Desc: "Board id or name (see 'opcli board list')"}
var pBoardFlag = Param{Name: "board", Kind: String, Short: "b", Required: true, Desc: "Board id or name (see 'opcli board list')"}
var pBoardProject = pProject("Project of the board, to find it by name (default: configured project, else every project)")

var boardList = &Op{
	Cmd:     []string{"board", "list"},
	Aliases: []string{"ls"},
	Short:   "List the boards of a project (or of every project)",
	Example: `  opcli board list -p demo-project
  opcli board list --project all -o json`,
	Params: []Param{pProject("Project id, identifier or name; 'all' for every project (default: configured project)")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "KIND", Key: "kind"}, {Header: "ATTRIBUTE", Key: "attribute"},
		{Header: "COLUMNS", Key: "columns"}, {Header: "PROJECT", Key: "project"}, {Header: "UPDATED", Key: "updatedAt"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		boards, err := e.boardsOf(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		items := []map[string]any{}
		for _, b := range boards {
			it := map[string]any{"id": b.ID, "name": b.Name, "project": b.Project, "kind": b.Kind,
				"columns": len(b.Columns), "createdAt": b.CreatedAt, "updatedAt": b.UpdatedAt, "url": e.boardURL(b)}
			if b.Kind == "action" {
				it["attribute"] = b.Attribute
			}
			items = append(items, it)
		}
		return NewList(items, len(items)), nil
	},
}

var boardGet = &Op{
	Cmd:     []string{"board", "get"},
	Aliases: []string{"view", "show"},
	Short:   "Show a board: its columns, left to right, with their cards in board order",
	Long: `Show a board as the web draws it: columns left to right and, in each, the
cards in their manual order (cards never placed by hand go last, by id).
Every card of a column is read to order it; --max only caps how many are
listed (total counts them all). --counts lists no cards.

Cards are compact: id, displayId, subject, type, status, assignee, priority,
dueDate, position (when placed by hand) and project (only when it is not the
board's, e.g. a subproject's).`,
	Example: `  opcli board get "Kanban" -p demo-project
  opcli board get 12 --column "In progress" --max 0 -o json
  opcli board get 12 --counts`,
	Params: []Param{pBoard, pBoardProject,
		{Name: "column", Kind: String, Desc: "Only this column (name or position)"},
		pMax(50),
		{Name: "counts", Kind: Bool, Desc: "Only the number of cards per column"},
	},
	Text: renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, err := e.findBoard(ctx, a.String("board"), a.String("project"))
		if err != nil {
			return nil, err
		}
		if err := e.loadColumns(ctx, b); err != nil {
			return nil, err
		}
		pid, err := e.ProjectID(ctx, b.Project)
		if err != nil {
			return nil, err
		}
		v := e.viewBoard(b)
		v.cards = !a.Bool("counts")
		only := -1
		if a.Has("column") {
			if only, err = b.findColumn(a.String("column")); err != nil {
				return nil, err
			}
			v.Columns = v.Columns[only : only+1]
		}
		max := a.Int("max")
		errs := make([]error, len(v.Columns))
		forEach(len(v.Columns), func(i int) {
			c := b.Columns[v.Columns[i].Position-1]
			cards, pos, total, err := e.columnCards(ctx, b, pid, c)
			if err != nil {
				errs[i] = fmt.Errorf("column %q: %w", c.Name, err)
				return
			}
			v.Columns[i].Total = &total
			if a.Bool("counts") {
				return
			}
			if max > 0 && len(cards) > max {
				cards = cards[:max]
			}
			out := make([]map[string]any, len(cards))
			for j, wp := range cards {
				out[j] = card(wp, pos, strconv.Itoa(pid))
			}
			v.Columns[i].Cards = out
		})
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
		return v, nil
	},
}

// --- board writes --------------------------------------------------------

// columnSpec is one column to create: its query name and filters.
type columnSpec struct {
	name    string
	value   string
	filters []any // widget filter JSON
}

// resolveColumn turns a value given by the user into a column of an action
// board (attr != "") or a free list.
func (e *Env) resolveColumn(ctx context.Context, attr string, pid int, ref string) (columnSpec, error) {
	if attr == "" || attr == "free" {
		if ref == "" {
			return columnSpec{}, fmt.Errorf("a free board column needs a name")
		}
		return columnSpec{name: ref, filters: []any{map[string]any{"manualSort": map[string]any{"operator": "ow", "values": []any{}}}}}, nil
	}
	ba := boardAttrs[attr]
	var id int
	var err error
	var name string
	switch attr {
	case "status":
		id, err = e.StatusID(ctx, ref)
	case "assignee":
		id, err = e.UserID(ctx, ref)
	case "version":
		id, err = e.VersionID(ctx, pid, ref)
	case "subproject":
		id, err = e.ProjectID(ctx, ref)
	case "subtasks":
		id, err = e.WorkPackage(ctx, ref)
	}
	if err != nil {
		return columnSpec{}, err
	}
	if id <= 0 {
		return columnSpec{}, fmt.Errorf("%s %q: a column needs a value, not none", attr, ref)
	}
	res, err := e.C.Get(ctx, href(ba.kind, id)[len("/api/v3"):], nil)
	if err != nil {
		return columnSpec{}, err
	}
	switch attr {
	case "subtasks":
		label := "#" + strconv.Itoa(id)
		if d := hal.String(res["displayId"]); d != "" && d != strconv.Itoa(id) {
			label = d
		}
		name = fmt.Sprintf("[%s] %s", label, hal.String(res["subject"]))
	default:
		name = hal.String(res["name"])
	}
	v := strconv.Itoa(id)
	return columnSpec{name: name, value: v,
		filters: []any{map[string]any{ba.filter: map[string]any{"operator": "=", "values": []any{v}}}}}, nil
}

// createColumnQuery creates the hidden query behind a column, as the web
// does: public, hidden, sorted by manual order then id.
func (e *Env) createColumnQuery(ctx context.Context, pid int, attr string, c columnSpec) (int, error) {
	var filters []any
	if attr == "" || attr == "free" {
		filters = []any{map[string]any{"_links": map[string]any{
			"filter":   hal.Ref("/api/v3/queries/filters/manualSort"),
			"operator": hal.Ref("/api/v3/queries/operators/ow"),
			"values":   []any{},
		}}}
	} else {
		ba := boardAttrs[attr]
		id, _ := strconv.Atoi(c.value)
		filters = []any{map[string]any{"_links": map[string]any{
			"filter":   hal.Ref("/api/v3/queries/filters/" + ba.filter),
			"operator": hal.Ref("/api/v3/queries/operators/="),
			"values":   []any{hal.Ref(href(ba.kind, id))},
		}}}
	}
	body := map[string]any{
		"name": c.name, "public": true, "hidden": true, "includeSubprojects": true,
		"filters": filters,
		"_links": map[string]any{
			"project": hal.Ref(href("projects", pid)),
			"sortBy": []any{
				hal.Ref("/api/v3/queries/sort_bys/manualSorting-asc"),
				hal.Ref("/api/v3/queries/sort_bys/id-asc"),
			},
		},
	}
	res, err := e.C.Post(ctx, "/queries", body)
	if err != nil {
		return 0, fmt.Errorf("column %q: %w", c.name, err)
	}
	return intOf(res["id"]), nil
}

// dropQueries deletes queries left behind by a failed write; what cannot be
// deleted is reported.
func (e *Env) dropQueries(ctx context.Context, ids []int) {
	var left []string
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if err := e.C.Delete(ctx, fmt.Sprintf("/queries/%d", id)); err != nil && !isNotFound(err) {
			left = append(left, strconv.Itoa(id))
		}
	}
	if len(left) > 0 {
		e.warnf("could not delete the hidden queries %s: remove them with 'opcli api request DELETE queries/<id> --yes'", strings.Join(left, ", "))
	}
}

func isNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.Status == 404
}

func widgetJSON(c *boardCol, i int) map[string]any {
	w := map[string]any{
		"_type": "GridWidget", "identifier": "work_package_query",
		"startRow": 1, "endRow": 2, "startColumn": i + 1, "endColumn": i + 2,
		"options": c.Options,
	}
	if c.WidgetID > 0 {
		w["id"] = c.WidgetID
	}
	return w
}

func widgetsJSON(cols []*boardCol) []any {
	out := make([]any, len(cols))
	for i, c := range cols {
		out[i] = widgetJSON(c, i)
	}
	return out
}

// saveColumns writes the board's columns in their new order.
func (e *Env) saveColumns(ctx context.Context, b *board, cols []*boardCol) error {
	_, err := e.C.Patch(ctx, fmt.Sprintf("/grids/%d", b.ID), map[string]any{
		"rowCount": 1, "columnCount": max(1, len(cols)), "widgets": widgetsJSON(cols),
	})
	return err
}

// afterWrite returns the board as it is now (no cards), or nothing in a dry run.
func (e *Env) afterWrite(ctx context.Context, id int) (any, error) {
	if e.DryRun {
		return &Message{OK: true, Message: "dry run"}, nil
	}
	res, err := e.C.Get(ctx, fmt.Sprintf("/grids/%d", id), nil)
	if err != nil {
		return nil, err
	}
	b, err := parseBoard(res)
	if err != nil {
		return nil, err
	}
	if err := e.loadColumns(ctx, b); err != nil {
		return nil, err
	}
	return e.viewBoard(b), nil
}

func (e *Env) defaultColumns(ctx context.Context, attr string, pid int) ([]string, error) {
	switch attr {
	case "free":
		return []string{"Unnamed list"}, nil
	case "status":
		items, _, err := e.C.Collect(ctx, "/statuses", nil, 0)
		if err != nil {
			return nil, err
		}
		for _, s := range items {
			if s["isDefault"] == true {
				return []string{strconv.Itoa(intOf(s["id"]))}, nil
			}
		}
	case "version":
		items, _, err := e.C.Collect(ctx, fmt.Sprintf("/projects/%d/versions", pid), nil, 0)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, it := range items {
			f := hal.Flatten(it)
			if f["status"] == "open" && intOf(f["definingProjectId"]) == pid {
				out = append(out, strconv.Itoa(intOf(it["id"])))
			}
		}
		return out, nil
	}
	return nil, nil
}

var boardCreate = &Op{
	Cmd:   []string{"board", "create"},
	Short: "Create a board: by an attribute (status, assignee, version, ...) or free",
	Long: `Create a board in a project. --attribute picks its kind:

  status, assignee, version, subproject, subtasks: one column per value of
    that field; moving a card between columns changes the field;
  free: columns are lists you fill with 'board card add'.

--columns gives the initial columns, left to right: values of the attribute
(status names, users, version names, subprojects, parent work packages) or,
in a free board, list names. Without it you get what the web creates: the
default status, the open versions of the project, one free list, or none.
Each column gets a hidden query of its own, as in the web.`,
	Example: `  opcli board create -p demo-project --name "Kanban" --attribute status --columns "New,In progress,Closed"
  opcli board create -p demo-project --name "Team" --attribute assignee --columns "jane,john"
  opcli board create -p demo-project --name "Releases" --attribute version
  opcli board create -p demo-project --name "Ideas" --attribute free --columns "Later,Next,Now"
  opcli board create -p demo-project --name "Epics" --attribute subtasks --columns "42,57" --dry-run`,
	Params: []Param{
		pProject(""),
		{Name: "name", Kind: String, Required: true, Desc: "Board name"},
		{Name: "attribute", Kind: String, Required: true, Enum: boardKinds, Desc: "Kind of board: " + strings.Join(boardKinds, ", ")},
		{Name: "columns", Kind: Strings, Desc: "Initial columns, comma-separated: attribute values, or list names in a free board"},
	},
	Safety: Write,
	Text:   renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		pid, err := e.ProjectID(ctx, a.String("project"))
		if err != nil {
			return nil, err
		}
		proj, err := e.C.Get(ctx, fmt.Sprintf("/projects/%d", pid), nil)
		if err != nil {
			return nil, err
		}
		attr := a.String("attribute")
		refs := a.Strings("columns")
		if !a.Has("columns") {
			if refs, err = e.defaultColumns(ctx, attr, pid); err != nil {
				return nil, err
			}
		}
		specs := make([]columnSpec, len(refs))
		for i, r := range refs {
			if specs[i], err = e.resolveColumn(ctx, attr, pid, r); err != nil {
				return nil, err
			}
		}
		var created []int
		var cols []*boardCol
		for _, s := range specs {
			qid, err := e.createColumnQuery(ctx, pid, attr, s)
			if err != nil {
				e.dropQueries(ctx, created)
				return nil, err
			}
			created = append(created, qid)
			cols = append(cols, &boardCol{QueryID: qid, Options: map[string]any{"queryId": qid, "filters": s.filters}})
		}
		opts := map[string]any{"type": "free"}
		if attr != "free" {
			opts = map[string]any{"type": "action", "attribute": attr}
		}
		res, err := e.C.Post(ctx, "/grids", map[string]any{
			"name": a.String("name"), "options": opts,
			"rowCount": 1, "columnCount": max(1, len(cols)), "widgets": widgetsJSON(cols),
			"_links": map[string]any{"scope": hal.Ref(fmt.Sprintf("/projects/%s/boards", hal.String(proj["identifier"])))},
		})
		if err != nil {
			e.dropQueries(ctx, created)
			return nil, err
		}
		return e.afterWrite(ctx, intOf(res["id"]))
	},
}

var boardUpdate = &Op{
	Cmd:     []string{"board", "update"},
	Aliases: []string{"rename"},
	Short:   "Rename a board",
	Example: `  opcli board update 12 --name "Team kanban"`,
	Params:  []Param{pBoard, pBoardProject, {Name: "name", Kind: String, Required: true, Desc: "New name"}},
	Safety:  Write,
	Text:    renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, err := e.findBoard(ctx, a.String("board"), a.String("project"))
		if err != nil {
			return nil, err
		}
		if _, err := e.C.Patch(ctx, fmt.Sprintf("/grids/%d", b.ID), map[string]any{"name": a.String("name")}); err != nil {
			return nil, err
		}
		return e.afterWrite(ctx, b.ID)
	},
}

var boardDelete = &Op{
	Cmd:   []string{"board", "delete"},
	Short: "Delete a board and its column queries (irreversible; the work packages stay)",
	Example: `  opcli board delete 12 --dry-run
  opcli board delete "Old kanban" -p demo-project --yes`,
	Params: []Param{pBoard, pBoardProject},
	Safety: Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, err := e.findBoard(ctx, a.String("board"), a.String("project"))
		if err != nil {
			return nil, err
		}
		if err := e.C.Delete(ctx, fmt.Sprintf("/grids/%d", b.ID)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("Board %d %q deleted with its %d columns", b.ID, b.Name, len(b.Columns))}, nil
	},
}

// --- column writes -------------------------------------------------------

func (e *Env) boardWithColumns(ctx context.Context, a Args) (*board, int, error) {
	b, err := e.findBoard(ctx, a.String("board"), a.String("project"))
	if err != nil {
		return nil, 0, err
	}
	if err := e.loadColumns(ctx, b); err != nil {
		return nil, 0, err
	}
	pid, err := e.ProjectID(ctx, b.Project)
	return b, pid, err
}

var pColumn = Param{Name: "column", Kind: String, Required: true, Desc: "Column name or position (1 = leftmost)"}
var pColPosition = Param{Name: "position", Kind: Int, Desc: "Place it here, 1 = leftmost (default: last)"}

var boardColumnAdd = &Op{
	Cmd:   []string{"board", "column", "add"},
	Short: "Add a column: a value of the board's attribute, or a list in a free board",
	Long: `Add a column to a board. In a board by attribute, --value is the value the
column shows (a status, a user, a version, a subproject or a parent work
package, by name or id), and the column takes its name. In a free board,
--name names the new list. A hidden query is created for it, as in the web;
if the board cannot be saved, the query is deleted again.`,
	Example: `  opcli board column add "Kanban" --value "On hold" --position 3
  opcli board column add 12 --value jane
  opcli board column add "Ideas" --name "Someday"`,
	Params: []Param{pBoard, pBoardProject,
		{Name: "value", Kind: String, Desc: "Attribute value of the column (boards by attribute)"},
		{Name: "name", Kind: String, Desc: "Column name (free boards; elsewhere it overrides the value's name)"},
		pColPosition,
	},
	Safety: Write,
	Text:   renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, pid, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		attr := "free"
		ref := a.String("name")
		if b.Kind == "action" {
			if _, ok := b.attr(); !ok {
				return nil, fmt.Errorf("board %q is by %q, which opcli cannot add columns to", b.Name, b.Attribute)
			}
			attr, ref = b.Attribute, a.String("value")
			if ref == "" {
				return nil, fmt.Errorf("board %q is by %s: pass --value", b.Name, attr)
			}
		} else if a.Has("value") {
			return nil, fmt.Errorf("board %q is a free board: pass --name, not --value", b.Name)
		}
		spec, err := e.resolveColumn(ctx, attr, pid, ref)
		if err != nil {
			return nil, err
		}
		if spec.value != "" {
			for _, c := range b.Columns {
				if c.Value == spec.value {
					return nil, fmt.Errorf("board %q already has a column for %q: %q", b.Name, spec.name, c.Name)
				}
			}
		}
		if b.Kind == "action" && a.String("name") != "" {
			spec.name = a.String("name")
		}
		at := len(b.Columns)
		if p := a.Int("position"); p >= 1 && p <= len(b.Columns) {
			at = p - 1
		}
		qid, err := e.createColumnQuery(ctx, pid, attr, spec)
		if err != nil {
			return nil, err
		}
		col := &boardCol{QueryID: qid, Options: map[string]any{"queryId": qid, "filters": spec.filters}}
		cols := append(append(append([]*boardCol{}, b.Columns[:at]...), col), b.Columns[at:]...)
		if err := e.saveColumns(ctx, b, cols); err != nil {
			e.dropQueries(ctx, []int{qid})
			return nil, err
		}
		return e.afterWrite(ctx, b.ID)
	},
}

var boardColumnRemove = &Op{
	Cmd:     []string{"board", "column", "remove"},
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a column and its hidden query (the work packages stay)",
	Long: `Remove a column from a board and delete its hidden query, so its manual
card order is lost. The work packages are not touched: in a board by
attribute they keep their value; in a free board they leave that list.`,
	Example: `  opcli board column remove "Kanban" --column "On hold" --dry-run
  opcli board column remove 12 --column 4 --yes`,
	Params: []Param{pBoard, pBoardProject, pColumn},
	Safety: Destructive,
	Text:   renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, _, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		i, err := b.findColumn(a.String("column"))
		if err != nil {
			return nil, err
		}
		gone := b.Columns[i]
		cols := append(append([]*boardCol{}, b.Columns[:i]...), b.Columns[i+1:]...)
		if err := e.saveColumns(ctx, b, cols); err != nil {
			return nil, err
		}
		e.dropQueries(ctx, []int{gone.QueryID})
		return e.afterWrite(ctx, b.ID)
	},
}

var boardColumnMove = &Op{
	Cmd:   []string{"board", "column", "move"},
	Short: "Move a column to another position",
	Example: `  opcli board column move "Kanban" --column "Closed" --position 5
  opcli board column move 12 --column 1 --position 2`,
	Params: []Param{pBoard, pBoardProject, pColumn,
		{Name: "position", Kind: Int, Required: true, Desc: "New position, 1 = leftmost"}},
	Safety: Write,
	Text:   renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, _, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		i, err := b.findColumn(a.String("column"))
		if err != nil {
			return nil, err
		}
		to := a.Int("position")
		if to < 1 || to > len(b.Columns) {
			return nil, fmt.Errorf("--position %d: the board has columns 1 to %d", to, len(b.Columns))
		}
		c := b.Columns[i]
		cols := append(append([]*boardCol{}, b.Columns[:i]...), b.Columns[i+1:]...)
		cols = append(cols[:to-1], append([]*boardCol{c}, cols[to-1:]...)...)
		if err := e.saveColumns(ctx, b, cols); err != nil {
			return nil, err
		}
		return e.afterWrite(ctx, b.ID)
	},
}

var boardColumnRename = &Op{
	Cmd:     []string{"board", "column", "rename"},
	Short:   "Rename a column (its hidden query)",
	Example: `  opcli board column rename "Ideas" --column "Later" --name "Someday"`,
	Params:  []Param{pBoard, pBoardProject, pColumn, {Name: "name", Kind: String, Required: true, Desc: "New name"}},
	Safety:  Write,
	Text:    renderBoard,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		b, _, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		i, err := b.findColumn(a.String("column"))
		if err != nil {
			return nil, err
		}
		if _, err := e.C.Patch(ctx, fmt.Sprintf("/queries/%d", b.Columns[i].QueryID), map[string]any{"name": a.String("name")}); err != nil {
			return nil, err
		}
		return e.afterWrite(ctx, b.ID)
	},
}

// --- card writes ---------------------------------------------------------

var cardPlaceParams = []Param{
	{Name: "top", Kind: Bool, Desc: "Put it first in the column"},
	{Name: "bottom", Kind: Bool, Desc: "Put it last in the column"},
	{Name: "before", Kind: String, Desc: "Put it just above this work package (id)"},
	{Name: "after", Kind: String, Desc: "Put it just below this work package (id)"},
}

// placement reads --top/--bottom/--before/--after; "" when none was given.
func placement(a Args) (string, string, error) {
	var set []string
	for _, k := range []string{"top", "bottom", "before", "after"} {
		if (k == "top" || k == "bottom") && a.Bool(k) || (k == "before" || k == "after") && a.String(k) != "" {
			set = append(set, k)
		}
	}
	switch len(set) {
	case 0:
		return "", "", nil
	case 1:
		return set[0], a.String(set[0]), nil
	}
	return "", "", fmt.Errorf("pass only one of --top, --bottom, --before, --after")
}

// placeIndex turns a placement into an index of order (the column's cards
// without the one being placed).
func (e *Env) placeIndex(ctx context.Context, order []int, where, ref, col string) (int, error) {
	switch where {
	case "top":
		return 0, nil
	case "", "bottom":
		return len(order), nil
	}
	other, err := e.WorkPackage(ctx, ref)
	if err != nil {
		return 0, err
	}
	for i, id := range order {
		if id == other {
			if where == "after" {
				i++
			}
			return i, nil
		}
	}
	return 0, fmt.Errorf("--%s %s: #%d is not in column %q", where, ref, other, col)
}

// place writes the order delta that puts id where asked in column c.
func (e *Env) place(ctx context.Context, b *board, pid int, c *boardCol, id int, where, ref string) (int, error) {
	cards, pos, _, err := e.columnCards(ctx, b, pid, c)
	if err != nil {
		return 0, err
	}
	var order []int
	for _, wp := range cards {
		if w := intOf(wp["id"]); w != id {
			order = append(order, w)
		}
	}
	at, err := e.placeIndex(ctx, order, where, ref, c.Name)
	if err != nil {
		return 0, err
	}
	return at + 1, e.patchOrder(ctx, c.QueryID, placeCard(order, pos, id, at))
}

// cardValue is the id of the board attribute's value on a work package.
func cardValue(wp map[string]any, ba boardAttr) string {
	links, _ := wp["_links"].(map[string]any)
	for _, name := range ba.wpLinks {
		switch l := links[name].(type) {
		case map[string]any:
			if id := hal.IDFromHref(hal.String(l["href"])); id != nil {
				return fmt.Sprint(id)
			}
		case []any:
			if len(l) > 0 {
				lm, _ := l[0].(map[string]any)
				if id := hal.IDFromHref(hal.String(lm["href"])); id != nil {
					return fmt.Sprint(id)
				}
			}
		}
	}
	return ""
}

type cardMoveResult struct {
	WP       int    `json:"wp"`
	Board    int    `json:"board"`
	From     string `json:"from,omitempty"`
	To       string `json:"to"`
	Changed  string `json:"changed,omitempty"`
	Value    string `json:"value,omitempty"`
	Position int    `json:"position,omitempty"`
}

func renderCardMove(w io.Writer, v any) error {
	r, ok := v.(*cardMoveResult)
	if !ok {
		return RenderJSON(w, v)
	}
	from := r.From
	if from == "" {
		from = "(off the board)"
	}
	fmt.Fprintf(w, "#%d: %s → %s", r.WP, from, r.To)
	if r.Position > 0 {
		fmt.Fprintf(w, ", position %d", r.Position)
	}
	if r.Changed != "" {
		fmt.Fprintf(w, " (%s changed)", r.Changed)
	}
	fmt.Fprintln(w)
	return nil
}

// freeColumnsOf lists the columns of a free board that hold id.
func (e *Env) freeColumnsOf(ctx context.Context, b *board, id int) ([]int, error) {
	var in []int
	for i, c := range b.Columns {
		pos, err := e.queryOrder(ctx, c.QueryID)
		if err != nil {
			return nil, err
		}
		if _, ok := pos[id]; ok {
			in = append(in, i)
		}
	}
	return in, nil
}

var boardCardMove = &Op{
	Cmd:   []string{"board", "card", "move"},
	Short: "Move a card to another column and/or to a place in its column",
	Long: `Drag a card. --to names the target column (default: the one it is in).

In a board by attribute, moving to another column changes the work
package's field to the column's value (status, assignee, version, project
or parent), through the same server-side validation as 'wp update': a
status the workflow does not allow fails listing the allowed ones. Check
'opcli wp allowed <id> --field status' first.

In a free board, the card leaves its list and joins the target one (pass
--from when it is in several).

--top, --bottom, --before <id> or --after <id> place it inside the target
column; without them it lands where the board puts new cards (last, unless
it was placed by hand before). Reordering inside one column needs one of
them.`,
	Example: `  opcli board card move 42 --board "Kanban" --to "In progress" --top
  opcli board card move 42 -b 12 --before 57
  opcli board card move 42 -b "Ideas" --to "Now" --bottom
  opcli board card move 42 -b "Kanban" --to Closed --notify=false --dry-run`,
	Params: append([]Param{
		{Name: "id", Kind: String, Required: true, Positional: true, Desc: "Work package id"},
		pBoardFlag, pBoardProject,
		{Name: "to", Kind: String, Desc: "Target column (name or position; default: its current column)"},
		{Name: "from", Kind: String, Desc: "Free boards: the list it leaves, when it is in several"},
		{Name: "notify", Kind: Bool, Default: true, Desc: "Send email notifications for the field change"},
	}, cardPlaceParams...),
	Safety: Write,
	Text:   renderCardMove,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		where, ref, err := placement(a)
		if err != nil {
			return nil, err
		}
		id, err := e.WorkPackage(ctx, a.String("id"))
		if err != nil {
			return nil, err
		}
		b, pid, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		if len(b.Columns) == 0 {
			return nil, fmt.Errorf("board %q has no columns", b.Name)
		}
		res := &cardMoveResult{WP: id, Board: b.ID}
		from := -1
		if b.Kind == "action" {
			ba, ok := b.attr()
			if !ok {
				return nil, fmt.Errorf("board %q is by %q, which opcli cannot move cards in", b.Name, b.Attribute)
			}
			wp, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
			if err != nil {
				return nil, err
			}
			cur := cardValue(wp, ba)
			for i, c := range b.Columns {
				if c.Value != "" && c.Value == cur {
					from = i
				}
			}
		} else {
			in, err := e.freeColumnsOf(ctx, b, id)
			if err != nil {
				return nil, err
			}
			switch {
			case a.Has("from"):
				f, err := b.findColumn(a.String("from"))
				if err != nil {
					return nil, err
				}
				if !containsInt(in, f) {
					return nil, fmt.Errorf("#%d is not in list %q", id, b.Columns[f].Name)
				}
				from = f
			case len(in) == 1:
				from = in[0]
			case len(in) > 1:
				var names []string
				for _, i := range in {
					names = append(names, fmt.Sprintf("%q", b.Columns[i].Name))
				}
				return nil, fmt.Errorf("#%d is in several lists (%s): pass --from", id, strings.Join(names, ", "))
			}
		}
		to := from
		if a.Has("to") {
			if to, err = b.findColumn(a.String("to")); err != nil {
				return nil, err
			}
		}
		if to < 0 {
			return nil, fmt.Errorf("#%d is not on board %q: pass --to", id, b.Name)
		}
		if from >= 0 {
			res.From = b.Columns[from].Name
		}
		res.To = b.Columns[to].Name
		if to == from && where == "" {
			return nil, fmt.Errorf("#%d is already in %q: pass --to, or --top/--bottom/--before/--after to reorder it", id, res.To)
		}
		if to != from {
			if b.Kind == "action" {
				ba, _ := b.attr()
				ua := Args{"id": strconv.Itoa(id), ba.wpArg: b.Columns[to].Value, "notify": a.Bool("notify")}
				if _, err := e.updateWP(ctx, ua, id); err != nil {
					return nil, err
				}
				res.Changed, res.Value = ba.wpArg, b.Columns[to].Value
			} else if from >= 0 {
				if err := e.patchOrder(ctx, b.Columns[from].QueryID, map[int]int{id: -1}); err != nil {
					return nil, err
				}
			}
		}
		if where != "" || (b.Kind == "free" && to != from) {
			if res.Position, err = e.place(ctx, b, pid, b.Columns[to], id, where, ref); err != nil {
				return nil, err
			}
		}
		return res, nil
	},
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

func freeBoardOnly(b *board) error {
	if b.Kind == "action" {
		return fmt.Errorf("board %q is by %s: its cards follow that field; use 'board card move --to' (or 'wp update')", b.Name, b.Attribute)
	}
	return nil
}

var boardCardAdd = &Op{
	Cmd:   []string{"board", "card", "add"},
	Short: "Put a work package on a list of a free board",
	Example: `  opcli board card add 42 --board "Ideas" --column "Next"
  opcli board card add 42 -b 12 --column 1 --top`,
	Params: append([]Param{
		{Name: "id", Kind: String, Required: true, Positional: true, Desc: "Work package id"},
		pBoardFlag, pBoardProject, pColumn,
	}, cardPlaceParams...),
	Safety: Write,
	Text:   renderCardMove,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		where, ref, err := placement(a)
		if err != nil {
			return nil, err
		}
		id, err := e.WorkPackage(ctx, a.String("id"))
		if err != nil {
			return nil, err
		}
		b, pid, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		if err := freeBoardOnly(b); err != nil {
			return nil, err
		}
		i, err := b.findColumn(a.String("column"))
		if err != nil {
			return nil, err
		}
		pos, err := e.queryOrder(ctx, b.Columns[i].QueryID)
		if err != nil {
			return nil, err
		}
		if _, ok := pos[id]; ok {
			return nil, fmt.Errorf("#%d is already in %q: use 'board card move' to reorder it", id, b.Columns[i].Name)
		}
		res := &cardMoveResult{WP: id, Board: b.ID, To: b.Columns[i].Name}
		if res.Position, err = e.place(ctx, b, pid, b.Columns[i], id, where, ref); err != nil {
			return nil, err
		}
		return res, nil
	},
}

var boardCardRemove = &Op{
	Cmd:     []string{"board", "card", "remove"},
	Aliases: []string{"rm"},
	Short:   "Take a work package off a list of a free board (the work package stays)",
	Example: `  opcli board card remove 42 --board "Ideas"
  opcli board card remove 42 -b 12 --column "Later"`,
	Params: []Param{
		{Name: "id", Kind: String, Required: true, Positional: true, Desc: "Work package id"},
		pBoardFlag, pBoardProject,
		{Name: "column", Kind: String, Desc: "The list to take it off (default: every list it is in)"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := e.WorkPackage(ctx, a.String("id"))
		if err != nil {
			return nil, err
		}
		b, _, err := e.boardWithColumns(ctx, a)
		if err != nil {
			return nil, err
		}
		if err := freeBoardOnly(b); err != nil {
			return nil, err
		}
		in, err := e.freeColumnsOf(ctx, b, id)
		if err != nil {
			return nil, err
		}
		if a.Has("column") {
			i, err := b.findColumn(a.String("column"))
			if err != nil {
				return nil, err
			}
			if !containsInt(in, i) {
				return nil, fmt.Errorf("#%d is not in %q", id, b.Columns[i].Name)
			}
			in = []int{i}
		}
		if len(in) == 0 {
			return nil, fmt.Errorf("#%d is not on board %q", id, b.Name)
		}
		var names []string
		for _, i := range in {
			if err := e.patchOrder(ctx, b.Columns[i].QueryID, map[int]int{id: -1}); err != nil {
				return nil, err
			}
			names = append(names, fmt.Sprintf("%q", b.Columns[i].Name))
		}
		return &Message{OK: true, Message: fmt.Sprintf("#%d taken off %s", id, strings.Join(names, ", "))}, nil
	},
}
