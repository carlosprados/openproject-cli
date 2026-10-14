package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthErrorsAndRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if u, p, _ := r.BasicAuth(); u != "apikey" || p != "secret" {
			t.Errorf("bad auth %q/%q", u, p)
		}
		switch r.URL.Path {
		case "/api/v3/flaky":
			if calls == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			fmt.Fprint(w, `{"ok":true}`)
		case "/api/v3/invalid":
			w.WriteHeader(422)
			fmt.Fprint(w, `{"_type":"Error","errorIdentifier":"urn:openproject-org:api:v3:errors:MultipleErrors","message":"Multiple field constraints have been violated.","_embedded":{"errors":[{"message":"Subject can't be blank."},{"message":"Type is invalid."}]}}`)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"_type":"Error","message":"nope"}`)
		}
	}))
	defer srv.Close()
	c := New(srv.URL+"/", "secret")

	res, err := c.Get(context.Background(), "flaky", nil)
	if err != nil || res["ok"] != true || calls != 2 {
		t.Fatalf("retry failed: %v %v calls=%d", res, err, calls)
	}

	_, err = c.Post(context.Background(), "/api/v3/invalid", map[string]any{})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 422 || len(apiErr.Details) != 2 {
		t.Fatalf("expected detailed APIError, got %#v", err)
	}
	if !IsNotFound(func() error { _, e := c.Get(context.Background(), "missing", nil); return e }()) {
		t.Error("expected IsNotFound")
	}
}

func TestCollectPaging(t *testing.T) {
	const total = 250
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		page, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		start := (page - 1) * size
		fmt.Fprintf(w, `{"_type":"Collection","total":%d,"_embedded":{"elements":[`, total)
		for i := start; i < min(start+size, total); i++ {
			if i > start {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%d}`, i+1)
		}
		fmt.Fprint(w, `]}}`)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")

	for max, want := range map[int]int{0: 250, 30: 30, 120: 120, 1000: 250} {
		items, tot, err := c.Collect(context.Background(), "/things", nil, max)
		if err != nil || len(items) != want || tot != total {
			t.Errorf("Collect(max=%d) = %d items, total %d, %v; want %d", max, len(items), tot, err, want)
		}
		if want > 0 && items[len(items)-1]["id"] != float64(want) {
			t.Errorf("Collect(max=%d): last id %v", max, items[len(items)-1]["id"])
		}
	}
}

// A server whose page ceiling is below MaxPageSize answers with its own
// pageSize: Collect must page by it, in order, a bounded number at a time.
func TestCollectFollowsServerPageSizeInParallel(t *testing.T) {
	const total, ceiling = 1050, 100
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		size = min(size, ceiling)
		page, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		start := (page - 1) * size
		fmt.Fprintf(w, `{"total":%d,"pageSize":%d,"_embedded":{"elements":[`, total, size)
		for i := start; i < min(start+size, total); i++ {
			if i > start {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%d}`, i+1)
		}
		fmt.Fprint(w, `]}}`)
	}))
	defer srv.Close()

	items, tot, err := New(srv.URL, "k").Collect(context.Background(), "/things", nil, 0)
	if err != nil || tot != total || len(items) != total {
		t.Fatalf("got %d items, total %d, %v", len(items), tot, err)
	}
	for i, it := range items {
		if it["id"] != float64(i+1) {
			t.Fatalf("item %d has id %v: pages out of order", i, it["id"])
		}
	}
	if p := peak.Load(); p > CollectParallel || p < 2 {
		t.Errorf("peak concurrency %d, want 2..%d", p, CollectParallel)
	}
}

func TestCollectFallbackRecoversFailedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if page == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"_type":"Error","message":"undefined method 'filters' for nil"}`)
			return
		}
		fmt.Fprintf(w, `{"total":3,"pageSize":1,"_embedded":{"elements":[{"id":%d}]}}`, page)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")

	if _, _, err := c.Collect(context.Background(), "/things", nil, 3); err == nil {
		t.Fatal("without a fallback a failed page must fail Collect")
	}
	var gotOffset string
	fb := func(_ context.Context, q url.Values, err error) ([]map[string]any, int, error) {
		if e, ok := err.(*APIError); !ok || e.Status != 500 {
			t.Errorf("fallback got %v", err)
		}
		gotOffset = q.Get("offset")
		return []map[string]any{{"id": float64(2), "recovered": true}}, 3, nil
	}
	items, _, err := c.CollectWith(context.Background(), "/things", nil, 3, fb)
	if err != nil || len(items) != 3 || items[1]["recovered"] != true || gotOffset != "2" {
		t.Fatalf("items %v, offset %q, %v", items, gotOffset, err)
	}
}

func TestURL(t *testing.T) {
	c := New("https://op.example.com/", "k")
	for in, want := range map[string]string{
		"work_packages/1":               "https://op.example.com/api/v3/work_packages/1",
		"/api/v3/projects":              "https://op.example.com/api/v3/projects",
		"https://other.example/x":       "https://other.example/x",
		"/api/v3/attachments/5/content": "https://op.example.com/api/v3/attachments/5/content",
	} {
		if got := c.URL(in, nil); got != want {
			t.Errorf("URL(%q) = %q, want %q", in, got, want)
		}
	}
}
