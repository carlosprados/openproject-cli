package ops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/carlosprados/openproject-cli/internal/client"
)

// routeOP answers "METHOD /path" from a table and records request bodies.
type routeOP struct {
	routes map[string]func(body map[string]any) (int, string)
	calls  []string
	bodies map[string]map[string]any
}

func newRouteEnv(t *testing.T, routes map[string]func(map[string]any) (int, string)) (*Env, *routeOP) {
	f := &routeOP{routes: routes, bodies: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewEnv(client.New(srv.URL, "k"), "4"), f
}

func (f *routeOP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/api/v3")
	f.calls = append(f.calls, key)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.bodies[key] = body
	w.Header().Set("Content-Type", "application/hal+json")
	h, ok := f.routes[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"_type":"Error","message":"not found"}`))
		return
	}
	status, out := h(body)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(out))
}

func reply(s string) func(map[string]any) (int, string) {
	return func(map[string]any) (int, string) { return 200, s }
}

var emptyForm = reply(`{"_type":"Form","_embedded":{"validationErrors":{},"schema":{}}}`)

func TestParseDateTime(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	now := time.Date(2026, 10, 13, 15, 20, 0, 0, madrid) // Tuesday
	for in, want := range map[string]string{
		"2026-10-20 10:00":     "2026-10-20T08:00:00Z",
		"2026-10-20T10:00":     "2026-10-20T08:00:00Z",
		"tomorrow 9:30":        "2026-10-14T07:30:00Z",
		"Tomorrow 9:30":        "2026-10-14T07:30:00Z",
		"16:00":                "2026-10-13T14:00:00Z",
		"+3d 16:00":            "2026-10-16T14:00:00Z",
		"2026-10-30":           "2026-10-30T08:00:00Z", // 09:00, after the switch to winter time
		"+2h":                  "2026-10-13T15:20:00Z",
		"+90m":                 "2026-10-13T14:50:00Z",
		"2026-10-20T10:00:00Z": "2026-10-20T10:00:00Z",
	} {
		got, err := parseDateTimeAt(in, now)
		if err != nil || apiTime(got) != want {
			t.Errorf("parseDateTimeAt(%q) = %s, %v; want %s", in, apiTime(got), err, want)
		}
	}
	for _, in := range []string{"", "next week", "25:00", "+soon"} {
		if _, err := parseDateTimeAt(in, now); err == nil {
			t.Errorf("parseDateTimeAt(%q): expected an error", in)
		}
	}
}

var meetingRoutes = map[string]func(map[string]any) (int, string){
	"GET /users/me":       reply(`{"id":4,"name":"Me"}`),
	"GET /my_preferences": reply(`{"timeZone":"Europe/Madrid"}`),
	"GET /principals":     reply(`{"_type":"Collection","total":1,"count":1,"_embedded":{"elements":[{"id":9,"name":"Jane Doe"}]}}`),
	"POST /meetings/form": emptyForm,
	"POST /meetings":      reply(`{"_type":"Meeting","id":7,"title":"Review"}`),
	"GET /meetings/7": reply(`{"_type":"Meeting","id":7,"lockVersion":2,
		"_links":{"participants":[{"href":"/api/v3/users/4"},{"href":"/api/v3/users/5"}]}}`),
	"POST /meetings/7/form": emptyForm,
	"PATCH /meetings/7":     reply(`{"_type":"Meeting","id":7}`),
}

func participants(body map[string]any) []string {
	var out []string
	for _, p := range body["_links"].(map[string]any)["participants"].([]any) {
		out = append(out, p.(map[string]any)["href"].(string))
	}
	return out
}

func TestMeetingCreateInvitesTheAuthorInTheirTimeZone(t *testing.T) {
	env, f := newRouteEnv(t, meetingRoutes)
	_, err := ByCmd("meeting create").Execute(context.Background(), env, Args{
		"title": "Review", "start": "2026-10-20 10:00", "duration": "45m", "participant": []string{"jane"}})
	if err != nil {
		t.Fatal(err)
	}
	post := f.bodies["POST /meetings"]
	if got := participants(post); !reflect.DeepEqual(got, []string{"/api/v3/users/4", "/api/v3/users/9"}) {
		t.Errorf("participants = %v", got)
	}
	if post["startTime"] != "2026-10-20T08:00:00Z" || post["duration"] != "PT45M" {
		t.Errorf("bad POST body %v", post)
	}
	if strings.Join(f.calls, ",") == "" || !strings.Contains(strings.Join(f.calls, ","), "POST /meetings/form,POST /meetings") {
		t.Errorf("the form must validate before the POST: %v", f.calls)
	}
}

func TestMeetingUpdateMergesParticipants(t *testing.T) {
	env, f := newRouteEnv(t, meetingRoutes)
	_, err := ByCmd("meeting update").Execute(context.Background(), env, Args{
		"id": "7", "participant": []string{"jane", "me"}, "remove_participant": []string{"5"}})
	if err != nil {
		t.Fatal(err)
	}
	patch := f.bodies["PATCH /meetings/7"]
	if got := participants(patch); !reflect.DeepEqual(got, []string{"/api/v3/users/4", "/api/v3/users/9"}) {
		t.Errorf("participants = %v", got)
	}
	if patch["lockVersion"] != float64(2) {
		t.Errorf("lockVersion not sent: %v", patch)
	}
}

func TestOutcomeOutsideAMeetingInProgressNamesTheFix(t *testing.T) {
	env, _ := newRouteEnv(t, map[string]func(map[string]any) (int, string){
		"POST /meeting_outcomes": func(map[string]any) (int, string) {
			return 422, `{"_type":"Error","errorIdentifier":"urn:openproject-org:api:v3:errors:PropertyConstraintViolation","message":"This outcome is not editable anymore."}`
		},
		"GET /meeting_agenda_items/37": reply(`{"id":37,"_links":{"meeting":{"href":"/api/v3/meetings/7"}}}`),
	})
	_, err := ByCmd("meeting outcome add").Execute(context.Background(), env, Args{"id": "37", "notes": "x"})
	if err == nil || !strings.Contains(err.Error(), "opcli meeting update 7 --state in_progress") {
		t.Errorf("expected the hint with the meeting id, got %v", err)
	}
}

func TestRemindMovesTheExistingReminder(t *testing.T) {
	env, f := newRouteEnv(t, map[string]func(map[string]any) (int, string){
		"GET /my_preferences": reply(`{"timeZone":"Europe/Madrid"}`),
		"POST /work_packages/42/reminders": func(map[string]any) (int, string) {
			return 409, `{"_type":"Error","message":"You can only set one reminder at a time for a work package."}`
		},
		"GET /work_packages/42/reminders": reply(`{"_type":"Collection","total":1,"count":1,"_embedded":{"elements":[{"id":3}]}}`),
		"PATCH /reminders/3":              reply(`{"_type":"Reminder","id":3,"remindAt":"2026-10-20T14:30:00.000Z"}`),
	})
	res, err := ByCmd("wp remind").Execute(context.Background(), env, Args{"id": "42", "at": "2026-10-20 16:30", "note": "call"})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.(map[string]any); r["replaced"] != true || r["id"] != float64(3) {
		t.Errorf("result %v", r)
	}
	if b := f.bodies["PATCH /reminders/3"]; b["remindAt"] != "2026-10-20T14:30:00Z" || b["note"] != "call" {
		t.Errorf("PATCH body %v", b)
	}
}

func TestTimeStopLogsTheElapsedTime(t *testing.T) {
	started := time.Now().Add(-90*time.Minute - 10*time.Second).UTC().Format(time.RFC3339)
	env, f := newRouteEnv(t, map[string]func(map[string]any) (int, string){
		"GET /users/me": reply(`{"id":4}`),
		"GET /time_entries": reply(`{"_type":"Collection","total":1,"count":1,"_embedded":{"elements":[
			{"id":11,"ongoing":true,"createdAt":"` + started + `","_links":{"entity":{"href":"/api/v3/work_packages/42","title":"S"}}}]}}`),
		"POST /time_entries/11/form": emptyForm,
		"PATCH /time_entries/11":     reply(`{"_type":"TimeEntry","id":11,"hours":"PT1H30M","ongoing":false}`),
	})
	if _, err := ByCmd("time stop").Execute(context.Background(), env, Args{"comment": "done"}); err != nil {
		t.Fatal(err)
	}
	b := f.bodies["PATCH /time_entries/11"]
	if b["hours"] != "PT1H30M" || b["ongoing"] != false || b["comment"].(map[string]any)["raw"] != "done" {
		t.Errorf("PATCH body %v", b)
	}
	if _, err := ByCmd("time start").Execute(context.Background(), env, Args{"wp": "43"}); err == nil || !strings.Contains(err.Error(), "already running on #42") {
		t.Errorf("expected the running timer error, got %v", err)
	}
}

func TestUpdateSeveralWorkPackagesReportsEachOutcome(t *testing.T) {
	env, _ := newTestEnv(t)
	_, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "42 #43", "subject": "x"})
	if err == nil || !strings.Contains(err.Error(), "1 of 2") || !strings.Contains(err.Error(), "#43:") || !strings.Contains(err.Error(), "updated: #42") {
		t.Errorf("expected a per-id report, got %v", err)
	}
	if _, err := workPackageIDs("42, x"); err == nil {
		t.Error("expected an invalid id error")
	}
}
