package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Work package fields as the server describes them. The form of a work
// package (POST …/form) is the truth for a write: it lists every field of the
// project and type, custom fields included, with the values each accepts
// right now (workflow transitions too), and it validates a payload without
// saving it.

var pField = Param{Name: "field", Kind: Strings, Desc: `Any field by its name, NAME=VALUE (repeatable): custom fields such as "Customer=ACME"; lists, users and versions by name; several values comma-separated; "NAME=" clears (see 'opcli wp fields')`}

type fieldDef struct {
	Key         string // customField3, subject, status…
	Name        string // as shown in the UI (localised)
	Type        string // String, Integer, CustomOption, []User…
	Required    bool
	Writable    bool
	InLinks     bool
	Allowed     []option // finite allowed values, when the schema embeds them
	AllowedHref string   // where to fetch them otherwise (users)
}

type option struct {
	ID   int
	Name string
	Href string
}

func (d fieldDef) custom() bool { return strings.HasPrefix(d.Key, "customField") }

func (d fieldDef) multi() bool { return strings.HasPrefix(d.Type, "[]") }

// parseSchema reads the field definitions of a work package schema.
func parseSchema(schema map[string]any) map[string]fieldDef {
	defs := map[string]fieldDef{}
	for key, raw := range schema {
		m, ok := raw.(map[string]any)
		if !ok || strings.HasPrefix(key, "_") {
			continue
		}
		typ, _ := m["type"].(string)
		if typ == "" {
			continue
		}
		d := fieldDef{
			Key:      key,
			Name:     hal.String(m["name"]),
			Type:     typ,
			Required: m["required"] == true,
			Writable: m["writable"] == true,
			InLinks:  m["location"] == "_links",
		}
		if emb, ok := m["_embedded"].(map[string]any); ok {
			if av, ok := emb["allowedValues"].([]any); ok {
				for _, v := range av {
					if o, ok := v.(map[string]any); ok {
						d.Allowed = append(d.Allowed, option{
							ID:   intOf(o["id"]),
							Name: hal.String(firstNonNil(o["value"], o["name"], o["title"], o["subject"])),
							Href: hal.Link(o, "self"),
						})
					}
				}
			}
		}
		if d.Allowed == nil {
			links, _ := m["_links"].(map[string]any)
			switch av := links["allowedValues"].(type) {
			case []any:
				for _, v := range av {
					if l, ok := v.(map[string]any); ok {
						h := hal.String(l["href"])
						d.Allowed = append(d.Allowed, option{ID: intOf(hal.IDFromHref(h)), Name: hal.String(l["title"]), Href: h})
					}
				}
			case map[string]any:
				d.AllowedHref = hal.String(av["href"])
			}
		}
		defs[key] = d
	}
	return defs
}

// lookupField finds a writable field by key or by its name.
func lookupField(defs map[string]fieldDef, ref string) (fieldDef, error) {
	ref = strings.TrimSpace(ref)
	for _, d := range defs {
		if d.Writable && (strings.EqualFold(d.Key, ref) || strings.EqualFold(d.Name, ref)) {
			return d, nil
		}
	}
	return fieldDef{}, fmt.Errorf("unknown field %q here; custom fields: %s (see 'opcli wp fields')", ref, strings.Join(customFieldNames(defs), ", "))
}

func customFieldNames(defs map[string]fieldDef) []string {
	var names []string
	for _, d := range defs {
		if d.custom() && d.Writable {
			names = append(names, d.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return []string{"none"}
	}
	return names
}

// fieldValue converts the text of --field into what the API expects for the
// field's type. Raw JSON (a value starting with [ or {) passes through.
func (e *Env) fieldValue(ctx context.Context, d fieldDef, raw string, projectID int) (any, error) {
	v := strings.TrimSpace(raw)
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
		var out any
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, fmt.Errorf("field %q: invalid JSON value: %w", d.Name, err)
		}
		return out, nil
	}
	if v == "" {
		switch {
		case d.multi():
			return []any{}, nil
		case d.InLinks:
			return hal.Ref(""), nil
		}
		return nil, nil
	}
	if d.InLinks {
		parts := []string{v}
		if d.multi() {
			parts = splitList(v)
		}
		var refs []any
		for _, p := range parts {
			h, err := e.fieldHref(ctx, d, p, projectID)
			if err != nil {
				return nil, err
			}
			refs = append(refs, hal.Ref(h))
		}
		if d.multi() {
			return refs, nil
		}
		return refs[0], nil
	}
	switch strings.TrimPrefix(d.Type, "[]") {
	case "Integer":
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("field %q expects a whole number, got %q", d.Name, v)
		}
		return n, nil
	case "Float":
		if !strings.Contains(v, ".") {
			v = strings.Replace(v, ",", ".", 1)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("field %q expects a number, got %q", d.Name, raw)
		}
		return f, nil
	case "Boolean":
		switch strings.ToLower(v) {
		case "true", "yes", "y", "1", "on", "si", "sí":
			return true, nil
		case "false", "no", "n", "0", "off":
			return false, nil
		}
		return nil, fmt.Errorf("field %q expects true or false, got %q", d.Name, v)
	case "Date":
		return ParseDate(v)
	case "Duration":
		h, err := hal.ParseHours(v)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", d.Name, err)
		}
		return hal.ISOHours(h), nil
	case "Formattable":
		return hal.Text(raw), nil
	}
	return v, nil
}

// fieldHref resolves one value of a link field: users and versions by
// name, anything else against the allowed values of the schema.
func (e *Env) fieldHref(ctx context.Context, d fieldDef, ref string, projectID int) (string, error) {
	switch strings.TrimPrefix(d.Type, "[]") {
	case "User":
		id, err := e.UserID(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("field %q: %w", d.Name, err)
		}
		return href("users", id), nil
	case "Version":
		if len(d.Allowed) == 0 {
			id, err := e.VersionID(ctx, projectID, ref)
			if err != nil {
				return "", fmt.Errorf("field %q: %w", d.Name, err)
			}
			return href("versions", id), nil
		}
	}
	opts := d.Allowed
	if opts == nil && d.AllowedHref != "" {
		items, _, err := e.C.Collect(ctx, d.AllowedHref, nil, 500)
		if err != nil {
			return "", err
		}
		for _, it := range items {
			opts = append(opts, option{ID: intOf(it["id"]), Name: hal.String(firstNonNil(it["value"], it["name"], it["title"])), Href: hal.Link(it, "self")})
		}
	}
	if o, ok := pickOption(opts, ref); ok {
		return o.Href, nil
	}
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.Name
	}
	if len(names) == 0 {
		return "", fmt.Errorf("field %q: %q is not allowed (no values are allowed here)", d.Name, ref)
	}
	return "", fmt.Errorf("field %q: %q is not allowed; allowed: %s", d.Name, ref, strings.Join(names, ", "))
}

// pickOption matches by id, exact name, or a unique partial name.
func pickOption(opts []option, ref string) (option, bool) {
	id, idErr := strconv.Atoi(ref)
	var partial []option
	for _, o := range opts {
		if (idErr == nil && o.ID == id) || strings.EqualFold(o.Name, ref) {
			return o, true
		}
		if strings.Contains(strings.ToLower(o.Name), strings.ToLower(ref)) {
			partial = append(partial, o)
		}
	}
	if len(partial) == 1 {
		return partial[0], true
	}
	return option{}, false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyFields sets the --field values on a create or update body.
func (e *Env) applyFields(ctx context.Context, body map[string]any, defs map[string]fieldDef, specs []string, projectID int) error {
	links := body["_links"].(map[string]any)
	for _, s := range specs {
		name, value, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("--field %q: expected NAME=VALUE", s)
		}
		d, err := lookupField(defs, name)
		if err != nil {
			return err
		}
		v, err := e.fieldValue(ctx, d, value, projectID)
		if err != nil {
			return err
		}
		if d.InLinks {
			links[d.Key] = v
		} else {
			body[d.Key] = v
		}
	}
	return nil
}

// form posts body to a work package form. It validates without saving and
// returns the schema the server applied plus the validation errors.
func (e *Env) form(ctx context.Context, path string, body map[string]any) (map[string]fieldDef, map[string]any, error) {
	res, err := e.C.Post(ctx, path, body)
	if err != nil {
		return nil, nil, err
	}
	emb, _ := res["_embedded"].(map[string]any)
	schema, _ := emb["schema"].(map[string]any)
	errs, _ := emb["validationErrors"].(map[string]any)
	return parseSchema(schema), errs, nil
}

// validate runs body through the form at path and turns validation errors
// into one error that also names the values each failing field accepts.
func (e *Env) validate(ctx context.Context, path string, body map[string]any) error {
	defs, errs, err := e.form(ctx, path, body)
	if err != nil {
		return err
	}
	return formError(defs, errs)
}

func formError(defs map[string]fieldDef, errs map[string]any) error {
	if len(errs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var details []string
	for _, k := range keys {
		m, _ := errs[k].(map[string]any)
		msg := hal.String(m["message"])
		if msg == "" {
			msg = k + " is invalid"
		}
		if d, ok := defs[k]; ok && len(d.Allowed) > 0 && len(d.Allowed) <= 50 {
			names := make([]string, len(d.Allowed))
			for i, o := range d.Allowed {
				names[i] = o.Name
			}
			msg += " Allowed: " + strings.Join(names, ", ") + "."
		}
		details = append(details, msg)
	}
	return &client.APIError{
		Status:     422,
		Identifier: "urn:openproject-org:api:v3:errors:PropertyConstraintViolation",
		Message:    "the server would reject this, nothing was written",
		Details:    details,
	}
}

// customFieldLabels names the custom fields of work packages: the API keys
// them customFieldN, which means nothing to a reader. It replaces those keys
// (and their …Id/…Ids twins) with a "customFields" object keyed by name.
func (e *Env) customFieldLabels(ctx context.Context, items []map[string]any) {
	for _, it := range items {
		var keys []string
		for k := range it {
			if strings.HasPrefix(k, "customField") && !strings.HasSuffix(k, "Id") && !strings.HasSuffix(k, "Ids") {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			continue
		}
		names := e.schemaNames(ctx, intOf(it["projectId"]), intOf(it["typeId"]))
		cf := map[string]any{}
		for _, k := range keys {
			v := it[k]
			delete(it, k)
			delete(it, k+"Id")
			delete(it, k+"Ids")
			if v == nil || v == "" {
				continue
			}
			if l, ok := v.([]any); ok && len(l) == 0 {
				continue
			}
			name := names[k]
			if name == "" || cf[name] != nil {
				name = k
			}
			cf[name] = v
		}
		if len(cf) > 0 {
			it["customFields"] = cf
		}
	}
}

// schemaNames maps customFieldN to names for a project and type (cached).
func (e *Env) schemaNames(ctx context.Context, projectID, typeID int) map[string]string {
	key := fmt.Sprintf("%d-%d", projectID, typeID)
	e.cache.mu.Lock()
	if n, ok := e.cache.schemas[key]; ok {
		e.cache.mu.Unlock()
		return n
	}
	e.cache.mu.Unlock()
	names := map[string]string{}
	if projectID > 0 && typeID > 0 {
		res, err := e.C.Get(ctx, "/work_packages/schemas/"+url.PathEscape(key), nil)
		if err == nil {
			for k, d := range parseSchema(res) {
				names[k] = d.Name
			}
		} else if ctx.Err() == nil {
			e.warnf("schema of project %d, type %d unavailable (%v): its custom fields keep their customFieldN keys", projectID, typeID, err)
		}
	}
	e.cache.mu.Lock()
	if e.cache.schemas == nil {
		e.cache.schemas = map[string]map[string]string{}
	}
	e.cache.schemas[key] = names
	e.cache.mu.Unlock()
	return names
}

// fieldFilters turns wp list --field NAME=VALUE into API filters. Names are
// resolved against the forms of the project's types; text fields match by
// "contains", everything else by equality.
func (e *Env) fieldFilters(ctx context.Context, f Filters, specs []string, projectID int) (Filters, error) {
	if len(specs) == 0 {
		return f, nil
	}
	if projectID == 0 {
		return nil, fmt.Errorf("--field needs a project (--project) to resolve field names")
	}
	types, err := e.C.Get(ctx, fmt.Sprintf("/projects/%d/types", projectID), nil)
	if err != nil {
		return nil, err
	}
	defs := map[string]fieldDef{}
	for _, t := range client.Elements(types) {
		body := map[string]any{"_links": map[string]any{"type": hal.Ref(href("types", intOf(t["id"])))}}
		d, _, err := e.form(ctx, fmt.Sprintf("/projects/%d/work_packages/form", projectID), body)
		if err != nil {
			return nil, err
		}
		for k, v := range d {
			if _, seen := defs[k]; !seen {
				defs[k] = v
			}
		}
	}
	for _, s := range specs {
		name, value, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("--field %q: expected NAME=VALUE", s)
		}
		d, err := lookupField(defs, name)
		if err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			f = f.Add(d.Key, "!*")
			continue
		}
		base := strings.TrimPrefix(d.Type, "[]")
		switch {
		case d.InLinks:
			var ids []any
			for _, p := range splitList(value) {
				h, err := e.fieldHref(ctx, d, p, projectID)
				if err != nil {
					return nil, err
				}
				ids = append(ids, hal.IDFromHref(h))
			}
			f = f.Add(d.Key, "=", ids...)
		case base == "String" || base == "Formattable" || base == "Link":
			f = f.Add(d.Key, "~", value)
		case base == "Boolean":
			v, err := e.fieldValue(ctx, d, value, projectID)
			if err != nil {
				return nil, err
			}
			f = f.Add(d.Key, "=", map[bool]string{true: "t", false: "f"}[v.(bool)])
		case base == "Date":
			day, err := ParseDate(value)
			if err != nil {
				return nil, err
			}
			f = f.Add(d.Key, "=d", day)
		default:
			f = f.Add(d.Key, "=", value)
		}
	}
	return f, nil
}

var wpFieldsOp = &Op{
	Cmd:   []string{"wp", "fields"},
	Short: "List the fields a work package takes: names, types, required, allowed values",
	Long: `List the writable fields of a work package as the server's form describes
them, custom fields included: the name to use with --field, the type, whether
it is required, the allowed values when they are a finite list, and the help
text configured in the instance.

With an id it describes that work package (statuses are the transitions the
workflow allows from its current status); without one it describes a new
work package of --type in --project.`,
	Example: `  opcli wp fields --project demo-project --type Task
  opcli wp fields 42
  opcli wp fields 42 --custom
  opcli wp fields -p demo-project -o json | jq -r '.items[] | select(.required) | .name'`,
	Params: []Param{
		{Name: "id", Kind: String, Positional: true, Desc: "Work package id (omit to describe a new one)"},
		pProject("Project of the new work package (default: configured project)"),
		{Name: "type", Kind: String, Short: "t", Desc: "Type of the new work package (default: as in wp create)"},
		{Name: "custom", Kind: Bool, Desc: "Only custom fields"},
	},
	Columns: []Column{
		{Header: "NAME", Key: "name"}, {Header: "TYPE", Key: "type"}, {Header: "REQUIRED", Key: "required"},
		{Header: "VALUES", Key: "values", Fmt: func(v any) string {
			l, _ := v.([]string)
			if len(l) > 8 {
				return strings.Join(l[:8], ", ") + ", …"
			}
			return strings.Join(l, ", ")
		}},
		{Header: "KEY", Key: "key"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		var path string
		body := map[string]any{"_links": map[string]any{}}
		if a.Has("id") && a.String("id") != "" {
			id, err := WorkPackageID(a.String("id"))
			if err != nil {
				return nil, err
			}
			cur, err := e.C.Get(ctx, fmt.Sprintf("/work_packages/%d", id), nil)
			if err != nil {
				return nil, err
			}
			path = fmt.Sprintf("/work_packages/%d/form", id)
			body["lockVersion"] = cur["lockVersion"]
		} else {
			pid, err := e.ProjectID(ctx, a.String("project"))
			if err != nil {
				return nil, err
			}
			tid := 0
			if a.Has("type") {
				tid, err = e.TypeID(ctx, a.String("type"))
			} else {
				tid, err = e.defaultTypeID(ctx, pid)
			}
			if err != nil {
				return nil, err
			}
			path = fmt.Sprintf("/projects/%d/work_packages/form", pid)
			body["_links"] = map[string]any{"type": hal.Ref(href("types", tid))}
		}
		defs, _, err := e.form(ctx, path, body)
		if err != nil {
			return nil, err
		}
		help := e.helpTexts(ctx)
		var items []map[string]any
		for _, d := range defs {
			if !d.Writable || (a.Bool("custom") && !d.custom()) || hiddenField[d.Key] {
				continue
			}
			it := map[string]any{"key": d.Key, "name": d.Name, "type": d.Type, "required": d.Required, "custom": d.custom()}
			if len(d.Allowed) > 0 {
				names := make([]string, len(d.Allowed))
				for i, o := range d.Allowed {
					names[i] = o.Name
				}
				it["values"] = names
			} else if d.AllowedHref != "" {
				it["valuesFrom"] = "users: opcli wp allowed <id> --field " + d.Key + ", or opcli project assignees"
			}
			if h := help[d.Key]; h != "" {
				it["help"] = h
			}
			items = append(items, it)
		}
		sort.Slice(items, func(i, j int) bool {
			ci, cj := items[i]["custom"].(bool), items[j]["custom"].(bool)
			if ci != cj {
				return cj // built-in fields first
			}
			return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
		})
		return NewList(items, len(items)), nil
	},
}

// Form fields that are not set through --field: they have their own flags
// or are handled by opcli.
var hiddenField = map[string]bool{"lockVersion": true, "attachments": true}

// helpTexts returns the instance's help texts for work package fields.
func (e *Env) helpTexts(ctx context.Context) map[string]string {
	out := map[string]string{}
	items, _, err := e.C.Collect(ctx, "/help_texts", nil, 0)
	if err != nil {
		return out
	}
	for _, it := range items {
		if hal.String(it["scope"]) != "WorkPackage" {
			continue
		}
		h := it["helpText"]
		if m, ok := h.(map[string]any); ok {
			h = m["raw"]
		}
		out[hal.String(it["attribute"])] = strings.TrimSpace(hal.String(h))
	}
	return out
}
