package ops

import (
	"context"
	"fmt"
	"net/url"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Reminders are personal: each user sees only their own, and has at most
// one active reminder per work package.

func init() {
	registerGroup(&Group{Name: "reminder", Aliases: []string{"reminders"}, Short: "Your work package reminders"})
	register(wpRemind, reminderList, reminderDelete)
}

var wpRemind = &Op{
	Cmd:   []string{"wp", "remind"},
	Short: "Remind yourself about a work package at a given time",
	Long: `Set a personal reminder: at that time OpenProject notifies you (in-app
and by email, as your notification settings say). Only you see it.

You can have one active reminder per work package: setting another one
moves the existing reminder to the new time (and note, if given). A bare
date means 09:00 that day; times are in your OpenProject time zone.`,
	Example: `  opcli wp remind 42 --at "tomorrow 9:00" --note "Call the customer"
  opcli wp remind 42 --at "2026-10-20 16:30"
  opcli wp remind 42 --at +2h`,
	Params: []Param{
		pID("Work package"),
		{Name: "at", Kind: String, Required: true, Desc: `When: "YYYY-MM-DD HH:MM", "tomorrow 9:00", "16:00", "+2h", a date (09:00)`},
		{Name: "note", Kind: String, Desc: "Why (shown in the notification)"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		t, err := ParseDateTime(a.String("at"), e.location(ctx))
		if err != nil {
			return nil, err
		}
		body := map[string]any{"remindAt": apiTime(t)}
		if a.Has("note") {
			body["note"] = a.String("note")
		}
		res, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/reminders", id), body)
		replaced := false
		if client.IsConflict(err) {
			existing, _, lerr := e.C.Collect(ctx, fmt.Sprintf("/work_packages/%d/reminders", id), nil, 1)
			if lerr != nil || len(existing) == 0 {
				return nil, err
			}
			res, err = e.C.Patch(ctx, fmt.Sprintf("/reminders/%d", intOf(existing[0]["id"])), body)
			replaced = true
		}
		if err != nil {
			return nil, err
		}
		r := flatReminder(res)
		if replaced {
			r["replaced"] = true
		}
		return r, nil
	},
}

var reminderList = &Op{
	Cmd:     []string{"reminder", "list"},
	Aliases: []string{"ls"},
	Short:   "List your active reminders",
	Example: `  opcli reminder list`,
	Params:  []Param{pMax(100)},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "REMIND AT", Key: "remindAt", Fmt: timeFmt}, {Header: "WP", Key: "remindableId"}, {Header: "WORK PACKAGE", Key: "remindable"}, {Header: "NOTE", Key: "note"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		l, err := e.list(ctx, "/reminders", nil, a)
		if err != nil {
			return nil, err
		}
		for _, it := range l.Items {
			trimReminder(it)
		}
		return l, nil
	},
}

var reminderDelete = &Op{
	Cmd:     []string{"reminder", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete one of your reminders",
	Example: `  opcli reminder delete 3 --yes`,
	Params:  []Param{pID("Reminder")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		if err := e.C.Delete(ctx, "/reminders/"+url.PathEscape(a.String("id"))); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted reminder " + a.String("id")}, nil
	},
}

func flatReminder(res map[string]any) map[string]any {
	return trimReminder(hal.Flatten(res))
}

func trimReminder(m map[string]any) map[string]any {
	for _, k := range []string{"_type", "creator", "creatorId"} {
		delete(m, k)
	}
	return m
}
