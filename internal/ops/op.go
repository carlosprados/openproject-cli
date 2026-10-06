// Package ops is the single source of truth for everything opcli can do.
//
// Each Op declares its parameters, help text, safety class and handler once.
// internal/cli turns every Op into a Cobra command (params → flags) and
// internal/mcpserver turns the very same Op into an MCP tool (params → JSON
// schema). CLI ↔ MCP parity therefore holds by construction: a capability
// cannot exist on one surface only, and flag names always match tool
// parameter names (kebab-case ↔ snake_case).
package ops

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/client"
)

// Kind is the type of a parameter.
type Kind int

const (
	String Kind = iota
	Int
	Bool
	Strings
)

// Param describes one input of an Op.
type Param struct {
	// Name in snake_case; the CLI flag is the kebab-case form.
	Name string
	Kind Kind
	Desc string
	// Required params are mandatory on both surfaces.
	Required bool
	// Positional params are taken from the first CLI argument (they can
	// also be passed as a flag). At most one per Op.
	Positional bool
	// Rest makes the last positional param swallow all remaining CLI
	// arguments, joined with spaces (free-text search).
	Rest    bool
	Default any
	// File adds a --<name>-file CLI flag that reads the value from a path
	// or from stdin ("-"), to avoid shell-escaping long Markdown.
	File  bool
	Enum  []string
	Short string
}

// Safety classifies what an Op does to the server.
type Safety int

const (
	Read Safety = iota
	Write
	Destructive
)

// Column describes a table column for text output.
type Column struct {
	Header string
	Key    string
	Fmt    func(any) string
}

// Op is one capability, exposed as a CLI command and an MCP tool.
type Op struct {
	Tool    string   // MCP tool name, e.g. op_list_work_packages
	Cmd     []string // CLI path, e.g. {"wp", "list"}
	Aliases []string
	Short   string
	Long    string
	Example string
	Params  []Param
	Safety  Safety
	// ReadOnlySafe keeps a non-Read Op available in read-only mode because
	// it enforces Env.ReadOnly itself (op_api_request).
	ReadOnlySafe bool
	// Columns drive the text table for List results.
	Columns []Column
	Run     func(ctx context.Context, e *Env, a Args) (any, error)
	// Text optionally overrides the default text rendering.
	Text func(w io.Writer, v any) error
}

// Group is a CLI parent command (e.g. "wp").
type Group struct {
	Name    string
	Aliases []string
	Short   string
	Long    string
}

// List is the result shape of every listing Op.
type List struct {
	Total int              `json:"total"`
	Count int              `json:"count"`
	Items []map[string]any `json:"items"`
}

// NewList wraps flattened items with the server total.
func NewList(items []map[string]any, total int) *List {
	if items == nil {
		items = []map[string]any{}
	}
	return &List{Total: total, Count: len(items), Items: items}
}

// Message is the result of Ops that only report an outcome.
type Message struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

var (
	registry []*Op
	groups   []*Group
)

func register(o ...*Op)      { registry = append(registry, o...) }
func registerGroup(g *Group) { groups = append(groups, g) }

// All returns every registered Op, sorted by CLI path.
func All() []*Op {
	out := append([]*Op(nil), registry...)
	sort.Slice(out, func(i, j int) bool {
		return strings.Join(out[i].Cmd, " ") < strings.Join(out[j].Cmd, " ")
	})
	return out
}

// Groups returns the registered CLI groups.
func Groups() []*Group { return groups }

// Env carries what handlers need: the API client, defaults and caches.
type Env struct {
	C              *client.Client
	DefaultProject string
	// DefaultType names the type of new work packages; empty means the
	// project's default type.
	DefaultType string
	// ReadOnly makes op_api_request refuse anything but GET (MCP --read-only).
	ReadOnly bool
	cache    resolverCache
}

// NewEnv builds an Env.
func NewEnv(c *client.Client, defaultProject string) *Env {
	return &Env{C: c, DefaultProject: defaultProject}
}

// Args holds parameter values by snake_case name. Only params explicitly
// set by the caller (or with a Default) are present.
type Args map[string]any

// Has reports whether name was provided.
func (a Args) Has(name string) bool {
	_, ok := a[name]
	return ok
}

// String returns name as a string (numbers are formatted).
func (a Args) String(name string) string {
	switch v := a[name].(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// Int returns name as an int (0 when absent or invalid).
func (a Args) Int(name string) int {
	switch v := a[name].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

// Bool returns name as a bool.
func (a Args) Bool(name string) bool {
	switch v := a[name].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	}
	return false
}

// Strings returns name as a string list; a single comma-separated string
// is split, so "a,b" and ["a","b"] are equivalent.
func (a Args) Strings(name string) []string {
	var raw []string
	switch v := a[name].(type) {
	case []string:
		raw = v
	case []any:
		for _, e := range v {
			raw = append(raw, fmt.Sprint(e))
		}
	case string:
		raw = []string{v}
	}
	var out []string
	for _, s := range raw {
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Validate checks required params and enums.
func (o *Op) Validate(a Args) error {
	for _, p := range o.Params {
		if p.Required && (!a.Has(p.Name) || a.String(p.Name) == "") {
			return fmt.Errorf("missing required parameter %q", p.Name)
		}
		if len(p.Enum) > 0 && a.Has(p.Name) {
			v := a.String(p.Name)
			ok := false
			for _, e := range p.Enum {
				if strings.EqualFold(e, v) {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("invalid %s %q (allowed: %s)", p.Name, v, strings.Join(p.Enum, ", "))
			}
		}
	}
	return nil
}

// Execute validates and runs the Op.
func (o *Op) Execute(ctx context.Context, e *Env, a Args) (any, error) {
	for _, p := range o.Params {
		if p.Default != nil && !a.Has(p.Name) {
			a[p.Name] = p.Default
		}
	}
	if err := o.Validate(a); err != nil {
		return nil, err
	}
	return o.Run(ctx, e, a)
}

// ByTool returns the Op with the given MCP tool name, or nil.
func ByTool(name string) *Op {
	for _, o := range registry {
		if o.Tool == name {
			return o
		}
	}
	return nil
}

// Positionals returns the positional params in declaration order.
func (o *Op) Positionals() []Param {
	var out []Param
	for _, p := range o.Params {
		if p.Positional {
			out = append(out, p)
		}
	}
	return out
}

// Flag returns the CLI flag name of a param.
func (p Param) Flag() string { return strings.ReplaceAll(p.Name, "_", "-") }
