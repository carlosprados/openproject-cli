package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/carlosprados/openproject-cli/internal/client"
)

func testDefs(t *testing.T) map[string]fieldDef {
	var schema map[string]any
	if err := json.Unmarshal([]byte(testSchema), &schema); err != nil {
		t.Fatal(err)
	}
	return parseSchema(schema)
}

func TestFieldValueConvertsByType(t *testing.T) {
	env, _ := newTestEnv(t)
	defs := testDefs(t)
	ctx := context.Background()
	for _, c := range []struct {
		def  fieldDef
		in   string
		want any
	}{
		{fieldDef{Name: "N", Type: "Integer"}, "8", 8},
		{fieldDef{Name: "F", Type: "Float"}, "3,5", 3.5},
		{fieldDef{Name: "B", Type: "Boolean"}, "sí", true},
		{fieldDef{Name: "B", Type: "Boolean"}, "no", false},
		{fieldDef{Name: "D", Type: "Date"}, "2026-01-02", "2026-01-02"},
		{fieldDef{Name: "W", Type: "Duration"}, "1h30m", "PT1H30M"},
		{fieldDef{Name: "T", Type: "Formattable"}, "**x**", map[string]any{"raw": "**x**"}},
		{fieldDef{Name: "S", Type: "String"}, "", nil},
		{fieldDef{Name: "S", Type: "String"}, `{"a":1}`, map[string]any{"a": float64(1)}},
		{defs["customField3"], "ops", map[string]any{"href": "/api/v3/custom_options/3"}},
		{defs["customField3"], "", map[string]any{"href": nil}},
		{defs["customField4"], "urgent, CUSTOMER", []any{map[string]any{"href": "/api/v3/custom_options/4"}, map[string]any{"href": "/api/v3/custom_options/5"}}},
		{defs["customField4"], "", []any{}},
	} {
		got, err := env.fieldValue(ctx, c.def, c.in, 4)
		if err != nil {
			t.Errorf("%s %q: %v", c.def.Type, c.in, err)
			continue
		}
		if !reflect.DeepEqual(normalise(got), normalise(c.want)) {
			t.Errorf("%s %q = %#v; want %#v", c.def.Type, c.in, got, c.want)
		}
	}
	if _, err := env.fieldValue(ctx, defs["customField3"], "Mobile", 4); err == nil || !strings.Contains(err.Error(), "allowed: Backend, Ops") {
		t.Errorf("expected the allowed values in the error, got %v", err)
	}
	if _, err := env.fieldValue(ctx, fieldDef{Name: "N", Type: "Integer"}, "many", 4); err == nil {
		t.Error("expected a number error")
	}
	if _, err := lookupField(defs, "Custmer"); err == nil || !strings.Contains(err.Error(), "Area, Customer, Points, Tags") {
		t.Errorf("expected the custom field names in the error, got %v", err)
	}
}

// normalise compares values through JSON (map[string]any vs hal.Ref).
func normalise(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func TestUpdateSetsFieldsByName(t *testing.T) {
	env, f := newTestEnv(t)
	_, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "42", "field": []string{"area=Ops", "Customer=Globex", "Tags=urgent,customer"}})
	if err != nil {
		t.Fatal(err)
	}
	var patch map[string]any
	forms := 0
	for i, r := range f.requests {
		if strings.Contains(r, "/form?") {
			forms++
		}
		if strings.HasPrefix(r, "PATCH") {
			patch = f.bodies[i]
		}
	}
	if forms < 2 {
		t.Errorf("expected the form to resolve and then validate, got %v", f.requests)
	}
	links := patch["_links"].(map[string]any)
	if patch["customField1"] != "Globex" || links["customField3"].(map[string]any)["href"] != "/api/v3/custom_options/3" || len(links["customField4"].([]any)) != 2 {
		t.Errorf("bad PATCH body %v", patch)
	}
}

func TestRejectedTransitionWritesNothing(t *testing.T) {
	env, f := newTestEnv(t)
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "42", "status": "New", "attach": []string{file}})
	if err == nil || !strings.Contains(err.Error(), "Allowed: In progress") || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("expected a validation error with the allowed statuses, got %v", err)
	}
	for _, r := range f.requests {
		if strings.HasPrefix(r, "PATCH") || strings.Contains(r, "/attachments") {
			t.Errorf("wrote despite the validation error: %s", r)
		}
	}
}

func TestDryRunRecordsWritesAndStillValidates(t *testing.T) {
	env, f := newTestEnv(t)
	rec := &client.Recorder{}
	env.C.DryRun = rec
	env.DryRun = true
	_, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "42", "status": "in progress", "comment": "done"})
	if err != nil {
		t.Fatal(err)
	}
	validated := false
	for _, r := range f.requests {
		if strings.HasPrefix(r, "PATCH") || strings.Contains(r, "/activities") {
			t.Errorf("dry run sent a write: %s", r)
		}
		validated = validated || strings.Contains(r, "/form?")
	}
	if !validated {
		t.Error("dry run skipped the server-side validation")
	}
	if len(rec.Requests) != 2 || rec.Requests[0].Method != "PATCH" || rec.Requests[1].Path != "/api/v3/work_packages/42/activities" {
		t.Errorf("recorded %+v", rec.Requests)
	}
}

func TestMentionsBecomeTags(t *testing.T) {
	env, _ := newTestEnv(t)
	m := &mentions{users: map[string]*mentionUser{}}
	in := "Ping @jane, not `@jane` nor jane@example.com; @nobody too. " +
		`<mention class="mention" data-id="9" data-type="user" data-text="@Jane Doe">@Jane Doe</mention>`
	got, err := env.linkMentions(context.Background(), in, m)
	if err != nil {
		t.Fatal(err)
	}
	tag := `<mention class="mention" data-id="9" data-type="user" data-text="@Jane Doe">@Jane Doe</mention>`
	want := "Ping " + tag + ", not `@jane` nor jane@example.com; @nobody too. " + tag
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if strings.Join(m.Mentioned, ",") != "Jane Doe" || strings.Join(m.Unresolved, ",") != "@nobody" {
		t.Errorf("mentioned %v, unresolved %v", m.Mentioned, m.Unresolved)
	}
}

func TestGetNamesCustomFields(t *testing.T) {
	env, _ := newTestEnv(t)
	res, err := ByCmd("wp get").Execute(context.Background(), env, Args{"id": "42"})
	if err != nil {
		t.Fatal(err)
	}
	wp := res.(map[string]any)
	cf, _ := wp["customFields"].(map[string]any)
	if cf["Customer"] != "ACME" {
		t.Errorf("customFields = %v", wp["customFields"])
	}
	if _, ok := wp["customField1"]; ok {
		t.Error("raw customField1 key should be replaced by its name")
	}
}
