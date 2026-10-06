package ops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carlosprados/openproject-cli/internal/client"
)

func TestDefaultTypeID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/hal+json")
		switch r.URL.Path {
		case "/api/v3/projects/4/types":
			_, _ = w.Write([]byte(`{"_type":"Collection","total":2,"count":2,"_embedded":{"elements":[{"id":3,"name":"Hito","isDefault":false},{"id":1,"name":"Tarea","isDefault":true}]}}`))
		case "/api/v3/projects/5/types":
			_, _ = w.Write([]byte(`{"_type":"Collection","total":1,"count":1,"_embedded":{"elements":[{"id":7,"name":"Bug","isDefault":false}]}}`))
		case "/api/v3/projects/6/types":
			_, _ = w.Write([]byte(`{"_type":"Collection","total":0,"count":0,"_embedded":{"elements":[]}}`))
		case "/api/v3/types":
			_, _ = w.Write([]byte(`{"_type":"Collection","total":2,"count":2,"_embedded":{"elements":[{"id":1,"name":"Tarea"},{"id":8,"name":"Incidencia"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	env := NewEnv(client.New(srv.URL, "k"), "")

	for pid, want := range map[int]int{4: 1, 5: 7} {
		if got, err := env.defaultTypeID(ctx, pid); err != nil || got != want {
			t.Errorf("project %d: got %d, %v; want %d", pid, got, err, want)
		}
	}
	if _, err := env.defaultTypeID(ctx, 6); err == nil {
		t.Error("expected error for a project without types")
	}
	env.DefaultType = "incidencia"
	if got, err := env.defaultTypeID(ctx, 4); err != nil || got != 8 {
		t.Errorf("configured default_type: got %d, %v; want 8", got, err)
	}
}
