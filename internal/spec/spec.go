// Package spec indexes the OpenProject API v3 OpenAPI document so every
// endpoint can be discovered and described offline (`opcli api endpoints`,
// `opcli api describe`, and their MCP tools). The embedded copy was taken
// from an OpenProject 17.8 instance; `Load` also accepts a live document
// fetched from /api/v3/spec.json.
package spec

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

//go:embed openapi.json.gz
var embedded []byte

// EmbeddedVersion is the OpenProject version the embedded spec comes from.
const EmbeddedVersion = "17.8.0"

var methods = []string{"get", "post", "patch", "put", "delete"}

// Spec is a parsed OpenAPI document.
type Spec struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

// Operation is the subset of an OpenAPI operation opcli exposes.
type Operation struct {
	Tags        []string    `json:"tags,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Description string      `json:"description,omitempty"`
	OperationID string      `json:"operationId,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty"`
	RequestBody *struct {
		Content map[string]struct {
			Schema   json.RawMessage            `json:"schema"`
			Examples map[string]json.RawMessage `json:"examples"`
		} `json:"content"`
	} `json:"requestBody,omitempty"`
}

// Parameter is an OpenAPI parameter.
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Schema      struct {
		Type    any `json:"type,omitempty"`
		Default any `json:"default,omitempty"`
	} `json:"schema"`
	Example any `json:"example,omitempty"`
}

// Endpoint is one method + path pair.
type Endpoint struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Tag     string `json:"tag"`
	Summary string `json:"summary"`
}

var (
	once      sync.Once
	cached    *Spec
	cachedErr error
)

// Embedded returns the spec compiled into the binary.
func Embedded() (*Spec, error) {
	once.Do(func() {
		data, err := gunzip(embedded)
		if err != nil {
			cachedErr = err
			return
		}
		cached, cachedErr = Load(data)
	})
	return cached, cachedErr
}

func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

// Load parses an OpenAPI JSON document.
func Load(data []byte) (*Spec, error) {
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI spec: %w", err)
	}
	return &s, nil
}

// Endpoints lists all endpoints whose path, tag or summary contains every
// word of search (case-insensitive). Empty search lists everything.
func (s *Spec) Endpoints(search string) []Endpoint {
	words := strings.Fields(strings.ToLower(search))
	var out []Endpoint
	for path, ops := range s.Paths {
		for _, m := range methods {
			raw, ok := ops[m]
			if !ok {
				continue
			}
			var op Operation
			if json.Unmarshal(raw, &op) != nil {
				continue
			}
			ep := Endpoint{Method: strings.ToUpper(m), Path: path, Summary: op.Summary}
			if len(op.Tags) > 0 {
				ep.Tag = op.Tags[0]
			}
			hay := strings.ToLower(ep.Method + " " + path + " " + ep.Tag + " " + op.Summary + " " + op.OperationID)
			match := true
			for _, w := range words {
				if !strings.Contains(hay, w) {
					match = false
					break
				}
			}
			if match {
				out = append(out, ep)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// Description is the answer of Describe.
type Description struct {
	Method       string        `json:"method"`
	Path         string        `json:"path"`
	Tag          string        `json:"tag,omitempty"`
	Summary      string        `json:"summary,omitempty"`
	Description  string        `json:"description,omitempty"`
	Parameters   []Parameter   `json:"parameters,omitempty"`
	BodySchema   string        `json:"bodySchema,omitempty"`
	BodyFields   []BodyField   `json:"bodyFields,omitempty"`
	BodyExamples []BodyExample `json:"bodyExamples,omitempty"`
}

// BodyField is one top-level property of a request body schema.
type BodyField struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
}

// BodyExample is a named request body example.
type BodyExample struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// Describe returns the operations matching path (a template such as
// /api/v3/work_packages/{id} or a concrete path such as
// /api/v3/work_packages/42). An empty method returns every method.
func (s *Spec) Describe(method, path string) ([]Description, error) {
	tmpl := s.matchPath(normalize(path))
	if tmpl == "" {
		return nil, fmt.Errorf("no endpoint matches %q (try `opcli api endpoints <words>`)", path)
	}
	var out []Description
	for _, m := range methods {
		if method != "" && !strings.EqualFold(method, m) {
			continue
		}
		raw, ok := s.Paths[tmpl][m]
		if !ok {
			continue
		}
		var op Operation
		if err := json.Unmarshal(raw, &op); err != nil {
			return nil, err
		}
		d := Description{Method: strings.ToUpper(m), Path: tmpl, Summary: op.Summary, Description: op.Description, Parameters: op.Parameters}
		if len(op.Tags) > 0 {
			d.Tag = op.Tags[0]
		}
		if rb := op.RequestBody; rb != nil {
			for _, c := range rb.Content {
				d.BodySchema, d.BodyFields = s.fields(c.Schema)
				for name, ex := range c.Examples {
					var e struct {
						Value json.RawMessage `json:"value"`
					}
					if json.Unmarshal(ex, &e) == nil && len(e.Value) > 0 {
						d.BodyExamples = append(d.BodyExamples, BodyExample{Name: name, Value: e.Value})
					}
				}
				break
			}
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s does not support %s", tmpl, strings.ToUpper(method))
	}
	return out, nil
}

func normalize(p string) string {
	p, _, _ = strings.Cut(p, "?")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, "/api/v3") {
		p = "/api/v3" + p
	}
	return strings.TrimRight(p, "/")
}

func (s *Spec) matchPath(p string) string {
	if _, ok := s.Paths[p]; ok {
		return p
	}
	want := strings.Split(p, "/")
	best, bestScore := "", -1
	for tmpl := range s.Paths {
		segs := strings.Split(tmpl, "/")
		if len(segs) != len(want) {
			continue
		}
		score := 0
		ok := true
		for i := range segs {
			switch {
			case segs[i] == want[i]:
				score += 2
			case strings.HasPrefix(segs[i], "{"):
				score++
			default:
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok && score > bestScore {
			best, bestScore = tmpl, score
		}
	}
	return best
}

// fields resolves a schema (following $ref and allOf) into its top-level
// properties.
func (s *Spec) fields(raw json.RawMessage) (string, []BodyField) {
	name := ""
	props := map[string]json.RawMessage{}
	var walk func(json.RawMessage, int)
	walk = func(r json.RawMessage, depth int) {
		if depth > 5 {
			return
		}
		var sc struct {
			Ref        string                     `json:"$ref"`
			AllOf      []json.RawMessage          `json:"allOf"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(r, &sc) != nil {
			return
		}
		if sc.Ref != "" {
			ref := strings.TrimPrefix(sc.Ref, "#/components/schemas/")
			if name == "" {
				name = ref
			}
			walk(s.Components.Schemas[ref], depth+1)
		}
		for _, a := range sc.AllOf {
			walk(a, depth+1)
		}
		for k, v := range sc.Properties {
			props[k] = v
		}
	}
	walk(raw, 0)
	var out []BodyField
	for k, v := range props {
		var p struct {
			Type        any    `json:"type"`
			Ref         string `json:"$ref"`
			Description string `json:"description"`
			ReadOnly    bool   `json:"readOnly"`
			Properties  map[string]json.RawMessage
		}
		_ = json.Unmarshal(v, &p)
		typ := fmt.Sprint(p.Type)
		if p.Type == nil {
			typ = strings.TrimPrefix(p.Ref, "#/components/schemas/")
		}
		desc := firstLine(p.Description)
		if k == "_links" && len(p.Properties) > 0 {
			names := make([]string, 0, len(p.Properties))
			for n := range p.Properties {
				names = append(names, n)
			}
			sort.Strings(names)
			desc = "links: " + strings.Join(names, ", ")
		}
		out = append(out, BodyField{Name: k, Type: typ, Description: desc, ReadOnly: p.ReadOnly})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return name, out
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}
