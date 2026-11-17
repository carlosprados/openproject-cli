package ops

import (
	"context"
	"strings"
	"testing"
)

// countRequests returns how many requests the fake server saw for path (any method).
func countRequests(f *fakeOP, path string) int {
	n := 0
	for _, r := range f.requests {
		if strings.Contains(r, " "+path+"?") {
			n++
		}
	}
	return n
}

func TestWorkPackageResolvesProjectBasedIdentifierOnceAndUpperCased(t *testing.T) {
	env, f := newTestEnv(t)
	for _, ref := range []string{"DEMO-42", "demo-42", " DEMO-42 "} {
		id, err := env.WorkPackage(context.Background(), ref)
		if err != nil || id != 42 {
			t.Fatalf("WorkPackage(%q) = %d, %v; want 42", ref, id, err)
		}
	}
	if n := countRequests(f, "/api/v3/work_packages/DEMO-42"); n != 1 {
		t.Errorf("the identifier should be resolved once per process, got %d requests: %v", n, f.requests)
	}
}

func TestWorkPackageNumericReferencesCostNoRequest(t *testing.T) {
	env, f := newTestEnv(t)
	for _, ref := range []string{"42", "#42", " 42 "} {
		id, err := env.WorkPackage(context.Background(), ref)
		if err != nil || id != 42 {
			t.Fatalf("WorkPackage(%q) = %d, %v; want 42", ref, id, err)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("numeric references must not reach the server, got %v", f.requests)
	}
}

func TestWorkPackageUnknownProjectBasedIdentifierSaysTheNumericIDWorks(t *testing.T) {
	env, _ := newTestEnv(t)
	_, err := env.WorkPackage(context.Background(), "DEMO-999")
	if err == nil || !strings.Contains(err.Error(), "DEMO-999 not found") || !strings.Contains(err.Error(), "numeric id") {
		t.Errorf("expected a not-found error pointing at the numeric id, got %v", err)
	}
}

func TestWorkPackageRejectsWhatIsNeitherFormWithoutARequest(t *testing.T) {
	env, f := newTestEnv(t)
	for _, ref := range []string{"", "x", "DEMO-", "-12", "DEMO-12a", "12-DEMO", "#DEMO-12", "DEMO 12"} {
		if _, err := env.WorkPackage(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "PROJ-123") {
			t.Errorf("WorkPackage(%q): expected an invalid-id error naming the accepted forms, got %v", ref, err)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid references must not reach the server, got %v", f.requests)
	}
}

func TestWorkPackageCommandsTakeProjectBasedIdentifiers(t *testing.T) {
	env, f := newTestEnv(t)
	res, err := ByCmd("wp get").Execute(context.Background(), env, Args{"id": "DEMO-42"})
	if err != nil {
		t.Fatal(err)
	}
	if wp, _ := res.(map[string]any); wp == nil || intOf(wp["id"]) != 42 {
		t.Errorf("wp get DEMO-42 returned %v", res)
	}
	if countRequests(f, "/api/v3/work_packages/42") == 0 {
		t.Errorf("wp get should fetch the resolved numeric id, requests: %v", f.requests)
	}
	if _, err := ByCmd("wp update").Execute(context.Background(), env, Args{"id": "DEMO-42", "subject": "x"}); err != nil {
		t.Errorf("wp update DEMO-42: %v", err)
	}
	if n := countRequests(f, "/api/v3/work_packages/DEMO-42"); n != 1 {
		t.Errorf("two commands in one process should resolve DEMO-42 once, got %d", n)
	}
}
