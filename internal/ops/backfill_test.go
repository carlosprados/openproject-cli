package ops

import (
	"context"
	"strings"
	"testing"
)

func TestSplitHours(t *testing.T) {
	days := []string{"2026-09-14", "2026-09-15", "2026-09-16"}
	got, err := splitHours(Args{"hours": "7h"}, days)
	if err != nil {
		t.Fatal(err)
	}
	// 28 quarters over 3 days: 10, 9, 9.
	want := []float64{2.5, 2.25, 2.25}
	for i, d := range got {
		if d.Hours != want[i] || d.Date != days[i] {
			t.Errorf("day %d = %+v, want %s %.2f", i, d, days[i], want[i])
		}
	}
	if got, _ := splitHours(Args{"hours": "30m"}, days); len(got) != 2 {
		t.Errorf("30m over 3 days should skip the empty day, got %+v", got)
	}
	if got, _ := splitHours(Args{"hours_per_day": "1h30m"}, days); len(got) != 3 || got[2].Hours != 1.5 {
		t.Errorf("per-day split wrong: %+v", got)
	}
	if _, err := splitHours(Args{"hours": "2h"}, nil); err == nil || !strings.Contains(err.Error(), "include-weekends") {
		t.Errorf("expected no-working-days error, got %v", err)
	}
}

func TestBackfillExistingWorkPackage(t *testing.T) {
	env, f := newTestEnv(t)
	// 2026-09-18 is a Friday and 2026-09-21 a Monday; /days is not served,
	// so the weekday fallback skips the weekend.
	res, err := ByTool("op_backfill_work_package").Execute(context.Background(), env, Args{
		"wp": "42", "from": "2026-09-18", "to": "2026-09-21", "hours": "3h", "reason": "Incident",
	})
	if err != nil {
		t.Fatal(err)
	}
	b := res.(*Backfill)
	if len(b.Days) != 2 || b.Days[0].Date != "2026-09-18" || b.Days[1].Date != "2026-09-21" || b.TotalHours != 3 {
		t.Errorf("unexpected days %+v", b.Days)
	}
	if b.Days[0].TimeEntryID == 0 || b.Days[1].TimeEntryID == 0 {
		t.Errorf("time entry ids not recorded: %+v", b.Days)
	}

	var patch, comment map[string]any
	var entries []map[string]any
	for i, r := range f.requests {
		switch {
		case strings.HasPrefix(r, "PATCH /api/v3/work_packages/42"):
			patch = f.bodies[i]
			if !strings.Contains(r, "notify=false") {
				t.Errorf("notifications should be off by default: %s", r)
			}
		case strings.HasPrefix(r, "POST /api/v3/time_entries"):
			entries = append(entries, f.bodies[i])
		case strings.HasPrefix(r, "POST /api/v3/work_packages/42/activities"):
			comment = f.bodies[i]
		}
	}
	if patch["startDate"] != "2026-09-18" || patch["dueDate"] != "2026-09-21" || patch["scheduleManually"] != true || patch["ignoreNonWorkingDays"] != false {
		t.Errorf("bad PATCH body %v", patch)
	}
	if len(entries) != 2 || entries[0]["spentOn"] != "2026-09-18" || entries[0]["hours"] != "PT1H30M" {
		t.Errorf("bad time entries %v", entries)
	}
	raw := comment["comment"].(map[string]any)["raw"].(string)
	for _, want := range []string{"Registered retroactively", "2026-09-18 → 2026-09-21", "3h logged over 2 days", "Reason: Incident"} {
		if !strings.Contains(raw, want) {
			t.Errorf("audit comment %q lacks %q", raw, want)
		}
	}
}

func TestBackfillValidation(t *testing.T) {
	env, f := newTestEnv(t)
	op := ByTool("op_backfill_work_package")
	for args, want := range map[*Args]string{
		{"from": "2026-09-14"}: "--subject",
		{"wp": "42", "from": "2026-09-14", "hours": "1h", "hours_per_day": "1h"}: "not both",
		{"wp": "42", "from": "2026-09-14", "to": "2026-09-10"}:                   "before",
		{"wp": "42", "from": "today", "to": "+3d"}:                               "future",
	} {
		if _, err := op.Execute(context.Background(), env, *args); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: expected error containing %q, got %v", *args, want, err)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("validation errors must not reach the server: %v", f.requests)
	}
}

func TestBackfillCommentLanguage(t *testing.T) {
	b := &Backfill{Start: "2026-09-14", Finish: "2026-09-15", TotalHours: 3, User: "jane",
		Days: []BackfillDay{{Date: "2026-09-14", Hours: 1.5, TimeEntryID: 7}, {Date: "2026-09-15", Hours: 1.5, TimeEntryID: 8}}}
	es := backfillComment(b, b.Days, "Incidencia", "ES")
	for _, want := range []string{"Registrado a posteriori", "2026-09-14 → 2026-09-15", "3h imputadas en 2 días a nombre de jane", "(imputaciones #7, #8)", "Motivo: Incidencia"} {
		if !strings.Contains(es, want) {
			t.Errorf("Spanish comment %q lacks %q", es, want)
		}
	}
	if en := backfillComment(b, b.Days, "", "fr"); !strings.HasPrefix(en, "**Registered retroactively**") || strings.Contains(en, "Reason") {
		t.Errorf("unknown language should fall back to English without a reason line: %q", en)
	}
}
