package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

func init() { register(wpBackfill) }

// Backfill is the result (or, with dry_run, the plan) of op_backfill_work_package.
type Backfill struct {
	DryRun      bool           `json:"dryRun,omitempty"`
	WorkPackage map[string]any `json:"workPackage,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	Start       string         `json:"startDate"`
	Finish      string         `json:"dueDate"`
	// IgnoreNonWorkingDays is set when the real dates touch a weekend or
	// holiday, which OpenProject otherwise rejects as start/finish.
	IgnoreNonWorkingDays bool          `json:"ignoreNonWorkingDays"`
	User                 string        `json:"user,omitempty"`
	TotalHours           float64       `json:"totalHours"`
	Days                 []BackfillDay `json:"days"`
	Comment              string        `json:"comment,omitempty"`
}

// BackfillDay is the time logged (or planned) on one day.
type BackfillDay struct {
	Date        string  `json:"date"`
	Hours       float64 `json:"hours"`
	TimeEntryID int     `json:"timeEntryId,omitempty"`
}

var wpBackfill = &Op{
	Tool:    "op_backfill_work_package",
	Cmd:     []string{"wp", "backfill"},
	Aliases: []string{"retro"},
	Short:   "Record work done after the fact: real dates, hours per day and an audit comment",
	Long: `Register work that already happened without being planned or logged.

In one call it creates the work package (--subject) or reuses an existing one
(--wp), sets its start/finish dates to the real ones (--from/--to, manual
scheduling), spreads --hours over the working days of that range (or logs
--hours-per-day on each), optionally moves it to --status, and adds a comment
recording that it was registered retroactively.

The creation date (createdAt) cannot be changed through the API; reports and
the Gantt should use start/finish dates and the time entries' spent date.

Working days come from the instance calendar (weekends and holidays are
skipped); --include-weekends logs on every day. Hours are split in 15-minute
steps, the remainder going to the first days. Time is logged for --user, else
the assignee, else you. Notifications are off by default. Use --dry-run to
see the plan without writing anything.`,
	Example: `  opcli wp backfill -p demo-project --subject "Hotfix MQTT broker" --assignee jane.doe \
    --from 2026-09-14 --to 2026-09-18 --hours 12h --activity Development --status Closed --dry-run
  opcli wp backfill --wp 42 --from 2026-09-21 --to 2026-09-23 --hours-per-day 2h \
    --reason "Done during the incident, not reported until today"
  opcli wp backfill --wp 42 --from -10d --to -8d     # only fix the dates, no time entries`,
	Params: []Param{
		{Name: "wp", Kind: String, Desc: "Existing work package id (omit and pass --subject to create one)"},
		{Name: "subject", Kind: String, Desc: "Title of the new work package (or new title for --wp)"},
		pProject("Project of the new work package (default: configured project)"),
		{Name: "type", Kind: String, Short: "t", Desc: "Type of the new work package (default: Task)"},
		{Name: "description", Kind: String, Short: "d", File: true, Desc: "Description in Markdown"},
		{Name: "assignee", Kind: String, Short: "a", Desc: "Who did the work: me, login, email, name or id"},
		{Name: "parent", Kind: String, Desc: "Parent work package id"},
		{Name: "status", Kind: String, Short: "s", Desc: "Final status, e.g. Closed (applied after logging time)"},
		{Name: "from", Kind: String, Required: true, Desc: "First real day of work: YYYY-MM-DD, yesterday, -Nd"},
		{Name: "to", Kind: String, Desc: "Last real day of work (default: same as --from)"},
		{Name: "hours", Kind: String, Short: "H", Desc: "Total time to spread over the working days: 12, 7.5, 1h30m"},
		{Name: "hours_per_day", Kind: String, Desc: "Time to log on each working day instead of a total"},
		{Name: "estimate", Kind: String, Desc: "Estimated work of a new work package (default: the logged total)"},
		{Name: "activity", Kind: String, Desc: "Time entry activity name or id (Development, Management, ...)"},
		{Name: "user", Kind: String, Short: "u", Desc: "Log time on behalf of this user (default: the assignee; needs permission)"},
		{Name: "comment", Kind: String, Short: "m", Desc: "Comment on each time entry (default: the subject)"},
		{Name: "reason", Kind: String, File: true, Desc: "Why it is registered late; appended to the audit comment"},
		{Name: "include_weekends", Kind: Bool, Desc: "Log time on every day of the range, not only working days"},
		{Name: "dry_run", Kind: Bool, Desc: "Show the plan without writing anything"},
		{Name: "notify", Kind: Bool, Default: false, Desc: "Send email notifications for these changes"},
	},
	Safety: Write,
	Run:    runBackfill,
	Text:   renderBackfill,
}

func runBackfill(ctx context.Context, e *Env, a Args) (any, error) {
	existing := a.String("wp")
	if existing == "" && a.String("subject") == "" {
		return nil, fmt.Errorf("pass --wp <id> to backfill an existing work package or --subject to create one")
	}
	if a.Has("hours") && a.Has("hours_per_day") {
		return nil, fmt.Errorf("use either --hours or --hours-per-day, not both")
	}
	from, err := ParseDate(a.String("from"))
	if err != nil {
		return nil, err
	}
	to := from
	if a.Has("to") {
		if to, err = ParseDate(a.String("to")); err != nil {
			return nil, err
		}
	}
	if to < from {
		return nil, fmt.Errorf("--to %s is before --from %s", to, from)
	}
	if today := time.Now().Format(time.DateOnly); to > today {
		return nil, fmt.Errorf("--to %s is in the future; backfill records work already done", to)
	}

	// Resolve what is used after the first write, so typos fail before it.
	if a.Has("status") {
		if _, err := e.StatusID(ctx, a.String("status")); err != nil {
			return nil, err
		}
	}
	if a.Has("activity") {
		wp, _ := WorkPackageID(existing) // 0 for a new one: instance-wide activities
		if _, err := e.ActivityID(ctx, wp, a.String("activity")); err != nil {
			return nil, err
		}
	}

	working, err := e.workingDays(ctx, from, to)
	if err != nil {
		return nil, err
	}
	logDays := working
	if a.Bool("include_weekends") {
		logDays = daysBetween(from, to)
	}
	days, err := splitHours(a, logDays)
	if err != nil {
		return nil, err
	}

	b := &Backfill{
		DryRun:               a.Bool("dry_run"),
		Subject:              a.String("subject"),
		Start:                from,
		Finish:               to,
		IgnoreNonWorkingDays: a.Bool("include_weekends") || !contains(working, from) || !contains(working, to),
		Days:                 days,
	}
	for _, d := range days {
		b.TotalHours += d.Hours
	}
	b.TotalHours = round2(b.TotalHours)

	userRef := a.String("user")
	if userRef == "" && a.Has("assignee") && !strings.EqualFold(a.String("assignee"), "none") {
		userRef = a.String("assignee")
	}
	b.User = userRef
	if b.DryRun {
		if existing != "" {
			b.WorkPackage = map[string]any{"id": existing}
		}
		b.Comment = backfillComment(b, nil, a.String("reason"))
		return b, nil
	}

	sched, _ := json.Marshal(map[string]any{"scheduleManually": true, "ignoreNonWorkingDays": b.IgnoreNonWorkingDays})
	wpArgs := Args{"start_date": from, "due_date": to, "fields": string(sched), "notify": a.Bool("notify")}
	for _, k := range []string{"subject", "description", "assignee", "parent"} {
		if a.Has(k) {
			wpArgs[k] = a[k]
		}
	}
	var res any
	if existing != "" {
		wpArgs["id"] = existing
		res, err = wpUpdate.Run(ctx, e, wpArgs)
	} else {
		wpArgs["project"] = a["project"]
		wpArgs["type"] = a["type"]
		if !a.Has("type") {
			delete(wpArgs, "type")
		}
		switch {
		case a.Has("estimate"):
			wpArgs["estimate"] = a["estimate"]
		case b.TotalHours > 0:
			wpArgs["estimate"] = strconv.FormatFloat(b.TotalHours, 'f', -1, 64)
		}
		res, err = wpCreate.Run(ctx, e, wpArgs)
	}
	if err != nil {
		return nil, err
	}
	b.WorkPackage = res.(map[string]any)
	id := intOf(b.WorkPackage["id"])
	if b.Subject == "" {
		b.Subject = hal.String(b.WorkPackage["subject"])
	}
	if userRef == "" {
		if aid := b.WorkPackage["assigneeId"]; aid != nil {
			userRef = hal.String(aid)
			b.User = hal.String(b.WorkPackage["assignee"])
		}
	}

	comment := a.String("comment")
	if comment == "" {
		comment = b.Subject
	}
	for i, d := range b.Days {
		ta := Args{"hours": strconv.FormatFloat(d.Hours, 'f', -1, 64), "date": d.Date, "comment": comment}
		if a.Has("activity") {
			ta["activity"] = a["activity"]
		}
		if userRef != "" {
			ta["user"] = userRef
		}
		body, err := e.timePayload(ctx, ta, id)
		if err == nil {
			body["_links"].(map[string]any)["entity"] = hal.Ref(href("work_packages", id))
			var te map[string]any
			if te, err = e.C.Post(ctx, "/time_entries", body); err == nil {
				b.Days[i].TimeEntryID = intOf(te["id"])
				continue
			}
		}
		return b, fmt.Errorf("work package #%d dated %s..%s, but logging time on %s failed after %d of %d entries (%s): %w",
			id, from, to, d.Date, i, len(b.Days), entryIDs(b.Days[:i]), err)
	}

	b.Comment = backfillComment(b, b.Days, a.String("reason"))
	up := Args{"id": strconv.Itoa(id), "comment": b.Comment, "notify": a.Bool("notify")}
	if a.Has("status") {
		up["status"] = a["status"]
	}
	if res, err = wpUpdate.Run(ctx, e, up); err != nil {
		return b, fmt.Errorf("work package #%d dated and %.2fh logged (%s), but the final update failed: %w",
			id, b.TotalHours, entryIDs(b.Days), err)
	}
	b.WorkPackage = res.(map[string]any)
	return b, nil
}

// workingDays returns the working days in [from, to] per the instance
// calendar, falling back to Monday–Friday if /days is unavailable.
func (e *Env) workingDays(ctx context.Context, from, to string) ([]string, error) {
	f := NewFilters().Add("date", "<>d", from, to).Add("working", "=", "t")
	items, _, err := e.C.Collect(ctx, "/days", url.Values{"filters": {f.JSON()}}, 0)
	if err != nil {
		var out []string
		for _, d := range daysBetween(from, to) {
			t, _ := time.Parse(time.DateOnly, d)
			if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
				out = append(out, d)
			}
		}
		return out, nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if d := hal.String(it["date"]); d != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

func daysBetween(from, to string) []string {
	f, _ := time.Parse(time.DateOnly, from)
	t, _ := time.Parse(time.DateOnly, to)
	var out []string
	for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(time.DateOnly))
	}
	return out
}

// splitHours turns --hours (a total, split in quarter hours with the
// remainder on the first days) or --hours-per-day into one entry per day.
func splitHours(a Args, days []string) ([]BackfillDay, error) {
	if !a.Has("hours") && !a.Has("hours_per_day") {
		return []BackfillDay{}, nil
	}
	if len(days) == 0 {
		return nil, fmt.Errorf("no working days between --from and --to; pass --include-weekends to log on them")
	}
	out := make([]BackfillDay, 0, len(days))
	if a.Has("hours_per_day") {
		h, err := hal.ParseHours(a.String("hours_per_day"))
		if err != nil {
			return nil, err
		}
		if h <= 0 {
			return nil, fmt.Errorf("hours per day must be positive")
		}
		for _, d := range days {
			out = append(out, BackfillDay{Date: d, Hours: h})
		}
		return out, nil
	}
	total, err := hal.ParseHours(a.String("hours"))
	if err != nil {
		return nil, err
	}
	quarters := int(math.Round(total * 4))
	if quarters <= 0 {
		return nil, fmt.Errorf("hours must be at least 15m")
	}
	base, extra := quarters/len(days), quarters%len(days)
	for i, d := range days {
		q := base
		if i < extra {
			q++
		}
		if q > 0 {
			out = append(out, BackfillDay{Date: d, Hours: float64(q) / 4})
		}
	}
	return out, nil
}

func backfillComment(b *Backfill, days []BackfillDay, reason string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**Registered retroactively** on %s. Real work dates: %s → %s.",
		time.Now().Format(time.DateOnly), b.Start, b.Finish)
	if len(b.Days) > 0 {
		fmt.Fprintf(&sb, " %sh logged over %d days", hal.String(b.TotalHours), len(b.Days))
		if b.User != "" {
			fmt.Fprintf(&sb, " for %s", b.User)
		}
		if ids := entryIDs(days); ids != "" {
			fmt.Fprintf(&sb, " (time entries %s)", ids)
		}
		sb.WriteString(".")
	}
	sb.WriteString(" The creation date reflects when it was recorded, not when the work happened.")
	if reason != "" {
		sb.WriteString("\n\nReason: " + reason)
	}
	return sb.String()
}

func entryIDs(days []BackfillDay) string {
	var ids []string
	for _, d := range days {
		if d.TimeEntryID > 0 {
			ids = append(ids, "#"+strconv.Itoa(d.TimeEntryID))
		}
	}
	return strings.Join(ids, ", ")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func renderBackfill(w io.Writer, v any) error {
	b := v.(*Backfill)
	if b.DryRun {
		fmt.Fprintln(w, "Dry run, nothing written. Plan:")
	}
	target := "new work package"
	if id := hal.String(b.WorkPackage["id"]); id != "" {
		target = "#" + id
	}
	if b.Subject != "" {
		target += " " + b.Subject
	}
	fmt.Fprintf(w, "Work package: %s\n", target)
	if u := hal.String(b.WorkPackage["url"]); u != "" {
		fmt.Fprintf(w, "URL:          %s\n", u)
	}
	fmt.Fprintf(w, "Dates:        %s → %s (manual scheduling", b.Start, b.Finish)
	if b.IgnoreNonWorkingDays {
		fmt.Fprint(w, ", non-working days included")
	}
	fmt.Fprintln(w, ")")
	if s := hal.String(b.WorkPackage["status"]); s != "" && !b.DryRun {
		fmt.Fprintf(w, "Status:       %s\n", s)
	}
	if len(b.Days) == 0 {
		fmt.Fprintln(w, "Time:         none logged")
	} else {
		user := b.User
		if user == "" {
			user = "me"
		}
		fmt.Fprintf(w, "Time:         %sh over %d days for %s\n\n", hal.String(b.TotalHours), len(b.Days), user)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "DATE\tHOURS\tENTRY")
		for _, d := range b.Days {
			entry := "-"
			if d.TimeEntryID > 0 {
				entry = "#" + strconv.Itoa(d.TimeEntryID)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Date, hal.String(d.Hours), entry)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if b.Comment != "" {
		fmt.Fprintf(w, "\nAudit comment:\n%s\n", b.Comment)
	}
	return nil
}
