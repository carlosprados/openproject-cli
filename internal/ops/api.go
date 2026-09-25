package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/hal"
	"github.com/carlosprados/openproject-cli/internal/spec"
)

// The generic escape hatch: any endpoint of API v3, discoverable offline
// through the embedded OpenAPI spec. Curated commands cover daily work;
// these three make sure nothing in the API is out of reach.

func init() {
	registerGroup(&Group{
		Name:  "api",
		Short: "Raw access to any API v3 endpoint, plus an offline endpoint catalogue",
		Long: `Anything the curated commands do not cover is reachable here:

  1. find the endpoint:     opcli api endpoints news
  2. read its contract:     opcli api describe POST /api/v3/news
  3. call it:               opcli api request POST /api/v3/news --body-file news.json

The catalogue comes from the OpenAPI spec embedded in the binary (OpenProject
` + spec.EmbeddedVersion + `); add --live to read the spec of the configured server instead.`,
	})
	register(apiRequest, apiEndpoints, apiDescribe)
}

var apiRequest = &Op{
	Tool:    "op_api_request",
	Cmd:     []string{"api", "request"},
	Aliases: []string{"call", "raw"},
	Short:   "Call any API v3 endpoint and print the JSON answer",
	Long: `Perform an authenticated request. The path may be relative to /api/v3
("work_packages/42") or absolute ("/api/v3/work_packages/42"). Query
parameters go in --query key=value (repeatable); JSON values such as
filters are passed verbatim. --flatten simplifies HAL answers (links become
"name"/"nameId" pairs, collections become their elements).

In read-only MCP mode only GET is allowed.`,
	Example: `  opcli api request GET /api/v3/configuration
  opcli api request GET work_packages --query 'filters=[{"status":{"operator":"o","values":[]}}]' --query pageSize=5 --flatten
  opcli api request POST /api/v3/render/markdown --body '"**bold**"'
  opcli api request PATCH work_packages/42 --body '{"lockVersion":3,"subject":"New"}'
  opcli api request POST news --body-file news.json`,
	Params: []Param{
		{Name: "method", Kind: String, Required: true, Positional: true, Enum: []string{"GET", "POST", "PATCH", "PUT", "DELETE"}, Desc: "HTTP method"},
		{Name: "path", Kind: String, Required: true, Positional: true, Desc: "Endpoint path, e.g. /api/v3/projects or projects/3"},
		{Name: "query", Kind: Strings, Desc: "Query parameter key=value (repeatable)"},
		{Name: "body", Kind: String, File: true, Desc: "JSON request body"},
		{Name: "flatten", Kind: Bool, Desc: "Simplify HAL output"},
	},
	Safety:       Destructive,
	ReadOnlySafe: true,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		method := strings.ToUpper(a.String("method"))
		if e.ReadOnly && method != http.MethodGet {
			return nil, fmt.Errorf("server is in read-only mode: only GET requests are allowed")
		}
		q := url.Values{}
		// Strings() splits on commas, which would break JSON values: read raw.
		for _, kv := range rawStrings(a["query"]) {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return nil, fmt.Errorf("invalid --query %q (want key=value)", kv)
			}
			q.Add(k, v)
		}
		var body []byte
		if b := a.String("body"); b != "" {
			if !json.Valid([]byte(b)) {
				return nil, fmt.Errorf("--body is not valid JSON")
			}
			body = []byte(b)
		}
		data, ct, err := e.C.Raw(ctx, method, a.String("path"), q, body, "application/json")
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return &Message{OK: true, Message: method + " " + a.String("path") + ": done (no content)"}, nil
		}
		if !strings.Contains(ct, "json") {
			return string(data), nil
		}
		var out any
		if err := json.Unmarshal(data, &out); err != nil {
			return string(data), nil
		}
		if m, ok := out.(map[string]any); ok && a.Bool("flatten") {
			if m["_type"] == "Collection" {
				return NewList(hal.FlattenAll(client.Elements(m)), intOf(m["total"])), nil
			}
			return hal.Flatten(m), nil
		}
		return out, nil
	},
	Text: func(w io.Writer, v any) error {
		if s, ok := v.(string); ok {
			_, err := fmt.Fprintln(w, s)
			return err
		}
		if m, ok := v.(*Message); ok {
			_, err := fmt.Fprintln(w, m.Message)
			return err
		}
		return RenderJSON(w, v)
	},
}

func rawStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = fmt.Sprint(e)
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	}
	return nil
}

func (e *Env) spec(ctx context.Context, live bool) (*spec.Spec, error) {
	if !live {
		return spec.Embedded()
	}
	data, _, err := e.C.Raw(ctx, http.MethodGet, "/api/v3/spec.json", nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fetching live spec: %w", err)
	}
	return spec.Load(data)
}

var pLive = Param{Name: "live", Kind: Bool, Desc: "Use the spec served by the configured instance instead of the embedded one"}

var apiEndpoints = &Op{
	Tool:    "op_api_list_endpoints",
	Cmd:     []string{"api", "endpoints"},
	Aliases: []string{"ls", "list"},
	Short:   "Search the catalogue of API v3 endpoints",
	Long:    `List endpoints whose method, path, tag or summary contain every given word.`,
	Example: `  opcli api endpoints                 # all ~300 operations
  opcli api endpoints meeting
  opcli api endpoints post work_packages
  opcli api endpoints grids --live`,
	Params: []Param{
		{Name: "search", Kind: String, Positional: true, Rest: true, Desc: "Words to match (all must match)"},
		pLive,
	},
	Columns: []Column{{Header: "METHOD", Key: "method"}, {Header: "PATH", Key: "path"}, {Header: "TAG", Key: "tag"}, {Header: "SUMMARY", Key: "summary"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		s, err := e.spec(ctx, a.Bool("live"))
		if err != nil {
			return nil, err
		}
		eps := s.Endpoints(a.String("search"))
		items := make([]map[string]any, len(eps))
		for i, ep := range eps {
			items[i] = map[string]any{"method": ep.Method, "path": ep.Path, "tag": ep.Tag, "summary": ep.Summary}
		}
		return NewList(items, len(items)), nil
	},
}

var apiDescribe = &Op{
	Tool:    "op_api_describe_endpoint",
	Cmd:     []string{"api", "describe"},
	Aliases: []string{"doc", "explain"},
	Short:   "Show parameters, body fields and examples of an endpoint",
	Long: `Describe an endpoint from the OpenAPI spec. The path can be the template
(/api/v3/work_packages/{id}) or a concrete path (/api/v3/work_packages/42).
Omit the method to describe every method of the path.`,
	Example: `  opcli api describe /api/v3/time_entries
  opcli api describe POST /api/v3/time_entries
  opcli api describe PATCH work_packages/42 -o json`,
	Params: []Param{
		{Name: "method_or_path", Kind: String, Required: true, Positional: true, Desc: "HTTP method, or the path when describing all methods"},
		{Name: "path", Kind: String, Positional: true, Desc: "Endpoint path (when a method is given first)"},
		pLive,
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		method, path := a.String("method_or_path"), a.String("path")
		if path == "" {
			method, path = "", method
		}
		s, err := e.spec(ctx, a.Bool("live"))
		if err != nil {
			return nil, err
		}
		return s.Describe(method, path)
	},
	Text: func(w io.Writer, v any) error {
		ds, ok := v.([]spec.Description)
		if !ok {
			return RenderJSON(w, v)
		}
		for i, d := range ds {
			if i > 0 {
				fmt.Fprintln(w, strings.Repeat("─", 60))
			}
			fmt.Fprintf(w, "%s %s   [%s]\n%s\n", d.Method, d.Path, d.Tag, d.Summary)
			if d.Description != "" {
				fmt.Fprintf(w, "\n%s\n", indent(truncate(d.Description, 1500)))
			}
			if len(d.Parameters) > 0 {
				fmt.Fprintln(w, "\nParameters:")
				for _, p := range d.Parameters {
					req := ""
					if p.Required {
						req = " (required)"
					}
					fmt.Fprintf(w, "  %-12s %-6s%s  %s\n", p.Name, p.In, req, firstLineOf(p.Description))
				}
			}
			if len(d.BodyFields) > 0 {
				fmt.Fprintf(w, "\nBody (%s):\n", d.BodySchema)
				for _, f := range d.BodyFields {
					ro := ""
					if f.ReadOnly {
						ro = " (read-only)"
					}
					fmt.Fprintf(w, "  %-22s %-12s%s  %s\n", f.Name, f.Type, ro, f.Description)
				}
			}
			for _, ex := range d.BodyExamples {
				fmt.Fprintf(w, "\nExample %q:\n%s\n", ex.Name, indent(string(ex.Value)))
			}
		}
		return nil
	},
}

func firstLineOf(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return truncate(s, 100)
}
