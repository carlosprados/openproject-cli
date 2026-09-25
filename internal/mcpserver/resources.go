package mcpserver

// MCP resources: read-only context an agent (or user) can attach without a
// tool round-trip. Each one is backed by an op that is also a tool.
//
//   openproject://guide                          usage guide (Markdown)
//   openproject://me                             authenticated user
//   openproject://catalog                        statuses, types, priorities, roles, time activities
//   openproject://projects                       active projects
//   openproject://my/work_packages               my open work packages, all projects
//   openproject://my/time_entries                my time entries this week
//   openproject://my/notifications               my unread notifications
//   openproject://api/endpoints                  catalogue of every API v3 endpoint
//   openproject://projects/{project}             one project
//   openproject://projects/{project}/work_packages  open work packages of a project
//   openproject://work_packages/{id}             one work package with relations, children, attachments
//   openproject://work_packages/{id}/activities  comments and change history

import (
	"context"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/guide"
	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResourceInfo describes a resource or template for listings.
type ResourceInfo struct {
	URI, Name, Description string
	Template               bool
}

type staticResource struct {
	ResourceInfo
	load func(ctx context.Context, env *ops.Env) (any, error)
}

func opLoader(tool string, args ops.Args) func(context.Context, *ops.Env) (any, error) {
	return func(ctx context.Context, env *ops.Env) (any, error) {
		a := ops.Args{}
		for k, v := range args {
			a[k] = v
		}
		return run(ctx, env, tool, a)
	}
}

var staticResources = []staticResource{
	{ResourceInfo{URI: "openproject://guide", Name: "Usage guide", Description: "How to use the OpenProject tools: concepts, name resolution, recipes, rules of thumb"},
		func(context.Context, *ops.Env) (any, error) { return guide.Text, nil }},
	{ResourceInfo{URI: "openproject://me", Name: "Current user", Description: "The authenticated OpenProject user"},
		opLoader("op_whoami", nil)},
	{ResourceInfo{URI: "openproject://catalog", Name: "Reference catalog", Description: "Statuses, types, priorities, roles and time-entry activities with ids"},
		loadCatalog},
	{ResourceInfo{URI: "openproject://projects", Name: "Projects", Description: "Active projects with id, identifier and name"},
		opLoader("op_list_projects", ops.Args{"max": 0})},
	{ResourceInfo{URI: "openproject://my/work_packages", Name: "My open work packages", Description: "Open work packages assigned to me in every project, most recently updated first"},
		opLoader("op_list_work_packages", ops.Args{"assignee": "me", "project": "all", "max": 200})},
	{ResourceInfo{URI: "openproject://my/time_entries", Name: "My time this week", Description: "My time entries since Monday with the hour total"},
		opLoader("op_list_time_entries", ops.Args{})},
	{ResourceInfo{URI: "openproject://my/notifications", Name: "My unread notifications", Description: "Unread in-app notifications (mentions, assignments...)"},
		opLoader("op_list_notifications", ops.Args{})},
	{ResourceInfo{URI: "openproject://api/endpoints", Name: "API endpoint catalogue", Description: "Every API v3 endpoint (method, path, summary) for use with op_api_request"},
		opLoader("op_api_list_endpoints", nil)},
}

type templateResource struct {
	ResourceInfo
	// prefix and suffix delimit the single variable of the URI template.
	prefix, suffix string
	load           func(ctx context.Context, env *ops.Env, v string) (any, error)
}

var templateResources = []templateResource{
	{ResourceInfo{URI: "openproject://projects/{project}", Name: "Project", Description: "One project by id or identifier", Template: true},
		"openproject://projects/", "",
		func(ctx context.Context, env *ops.Env, v string) (any, error) {
			return run(ctx, env, "op_get_project", ops.Args{"project": v})
		}},
	{ResourceInfo{URI: "openproject://projects/{project}/work_packages", Name: "Project work packages", Description: "Open work packages of a project", Template: true},
		"openproject://projects/", "/work_packages",
		func(ctx context.Context, env *ops.Env, v string) (any, error) {
			return run(ctx, env, "op_list_work_packages", ops.Args{"project": v, "max": 200})
		}},
	{ResourceInfo{URI: "openproject://work_packages/{id}", Name: "Work package", Description: "One work package with description, relations, children and attachments", Template: true},
		"openproject://work_packages/", "",
		func(ctx context.Context, env *ops.Env, v string) (any, error) {
			return run(ctx, env, "op_get_work_package", ops.Args{"id": v})
		}},
	{ResourceInfo{URI: "openproject://work_packages/{id}/activities", Name: "Work package activity", Description: "Comments and change history of a work package", Template: true},
		"openproject://work_packages/", "/activities",
		func(ctx context.Context, env *ops.Env, v string) (any, error) {
			return run(ctx, env, "op_list_work_package_activities", ops.Args{"id": v})
		}},
}

// Resources lists every resource and template (for `opcli mcp resources`).
func Resources() []ResourceInfo {
	var out []ResourceInfo
	for _, r := range staticResources {
		out = append(out, r.ResourceInfo)
	}
	for _, t := range templateResources {
		out = append(out, t.ResourceInfo)
	}
	return out
}

func loadCatalog(ctx context.Context, env *ops.Env) (any, error) {
	out := map[string]any{}
	for key, tool := range map[string]string{
		"statuses": "op_list_statuses", "types": "op_list_types", "priorities": "op_list_priorities",
		"roles": "op_list_roles", "timeActivities": "op_list_time_activities",
	} {
		res, err := run(ctx, env, tool, nil)
		if err != nil {
			return nil, err
		}
		if l, ok := res.(*ops.List); ok {
			out[key] = l.Items
		}
	}
	return out, nil
}

func registerResources(s *mcp.Server, env *ops.Env) {
	for _, r := range staticResources {
		mime := "application/json"
		if r.URI == "openproject://guide" {
			mime = "text/markdown"
		}
		s.AddResource(&mcp.Resource{URI: r.URI, Name: r.Name, Description: r.Description, MIMEType: mime},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				v, err := r.load(ctx, env)
				if err != nil {
					return nil, err
				}
				return contents(req.Params.URI, mime, v)
			})
	}
	// Longest prefix+suffix first so ".../work_packages" beats the bare project template.
	for _, t := range templateResources {
		s.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: t.URI, Name: t.Name, Description: t.Description, MIMEType: "application/json"},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				uri := req.Params.URI
				for _, cand := range templateResources {
					v, ok := match(uri, cand.prefix, cand.suffix)
					if ok {
						res, err := cand.load(ctx, env, v)
						if err != nil {
							return nil, err
						}
						return contents(uri, "application/json", res)
					}
				}
				return nil, mcp.ResourceNotFoundError(uri)
			})
	}
}

// match extracts the variable between prefix and suffix; the variable may
// not contain "/".
func match(uri, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return "", false
	}
	v := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), suffix)
	if v == "" || strings.Contains(v, "/") {
		return "", false
	}
	return v, true
}

func contents(uri, mime string, v any) (*mcp.ReadResourceResult, error) {
	text, err := toText(v)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mime, Text: text}}}, nil
}
