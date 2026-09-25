package spec

import (
	"strings"
	"testing"
)

func TestEmbeddedSpec(t *testing.T) {
	s, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Endpoints("")); n < 200 {
		t.Fatalf("only %d endpoints in embedded spec", n)
	}
	eps := s.Endpoints("post time_entries")
	if len(eps) == 0 || eps[0].Method != "POST" {
		t.Errorf("search failed: %+v", eps)
	}

	ds, err := s.Describe("PATCH", "/api/v3/work_packages/42")
	if err != nil {
		t.Fatal(err)
	}
	if ds[0].Path != "/api/v3/work_packages/{id}" || len(ds[0].BodyFields) == 0 {
		t.Errorf("concrete path not matched to template: %+v", ds[0])
	}
	all, err := s.Describe("", "time_entries")
	if err != nil || len(all) != 2 {
		t.Errorf("expected GET+POST for /time_entries, got %d, %v", len(all), err)
	}
	if _, err := s.Describe("", "/api/v3/nope/nope"); err == nil {
		t.Error("expected no-match error")
	}
}

// The embedded spec must not leak the host it was downloaded from.
func TestEmbeddedSpecIsSanitised(t *testing.T) {
	if !strings.Contains(string(mustRaw(t)), `"url":"https://openproject.example.com/"`) {
		t.Error("servers[].url must be the example placeholder")
	}
}

func mustRaw(t *testing.T) []byte {
	t.Helper()
	data, err := gunzip(embedded)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
