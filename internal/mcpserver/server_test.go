package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect starts the server against a fake OpenProject and returns a client session.
func connect(t *testing.T, readOnly bool) *mcp.ClientSession {
	t.Helper()
	op := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/hal+json")
		switch r.URL.Path {
		case "/api/v3/users/me":
			fmt.Fprint(w, `{"_type":"User","id":8,"name":"Test User","login":"tu"}`)
		case "/api/v3/work_packages/42":
			fmt.Fprint(w, `{"_type":"WorkPackage","id":42,"subject":"Hello","_links":{"status":{"href":"/api/v3/statuses/1","title":"New"}}}`)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"_type":"Error","message":"not found"}`)
		}
	}))
	t.Cleanup(op.Close)

	env := ops.NewEnv(client.New(op.URL, "k"), "")
	srv := New(env, Options{Version: "test", ReadOnly: readOnly})
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestToolsMirrorOps(t *testing.T) {
	cs := connect(t, false)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(ops.All()) {
		t.Errorf("%d tools for %d ops", len(res.Tools), len(ops.All()))
	}
	for _, tool := range res.Tools {
		op := ops.ByTool(tool.Name)
		if op == nil {
			t.Errorf("tool %s has no op", tool.Name)
			continue
		}
		if !strings.Contains(tool.Description, "opcli "+strings.Join(op.Cmd, " ")) {
			t.Errorf("%s: description lacks CLI equivalent", tool.Name)
		}
		if strings.Contains(tool.Description, "--") {
			t.Errorf("%s: description still mentions CLI flags: %q", tool.Name, tool.Description)
		}
	}
}

func TestReadOnlyHidesWriters(t *testing.T) {
	cs := connect(t, true)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if !tool.Annotations.ReadOnlyHint && tool.Name != "op_api_request" {
			t.Errorf("read-only server exposes %s", tool.Name)
		}
	}
}

func TestCallToolAndErrors(t *testing.T) {
	cs := connect(t, false)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "op_get_work_package", Arguments: map[string]any{"id": 42}})
	if err != nil || res.IsError {
		t.Fatalf("call failed: %v %+v", err, res)
	}
	var wp map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &wp); err != nil {
		t.Fatal(err)
	}
	if wp["subject"] != "Hello" || wp["status"] != "New" {
		t.Errorf("unexpected payload %v", wp)
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "op_get_work_package", Arguments: map[string]any{"id": 7}})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "404") {
		t.Errorf("API errors must be tool results with IsError: %v %+v", err, res)
	}
}

func TestResourcesAndPrompts(t *testing.T) {
	cs := connect(t, false)
	ctx := context.Background()

	r, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "openproject://work_packages/42"})
	if err != nil || !strings.Contains(r.Contents[0].Text, `"subject":"Hello"`) {
		t.Errorf("template resource: %v %+v", err, r)
	}
	r, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "openproject://me"})
	if err != nil || !strings.Contains(r.Contents[0].Text, "Test User") {
		t.Errorf("static resource: %v %+v", err, r)
	}
	g, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "draft_work_package", Arguments: map[string]string{}})
	if err == nil {
		t.Errorf("missing required argument should fail, got %+v", g)
	}
}

func TestMatch(t *testing.T) {
	if v, ok := match("openproject://projects/demo/work_packages", "openproject://projects/", "/work_packages"); !ok || v != "demo" {
		t.Errorf("got %q %v", v, ok)
	}
	if _, ok := match("openproject://projects/demo/work_packages", "openproject://projects/", ""); ok {
		t.Error("bare project template must not swallow sub-resources")
	}
}
