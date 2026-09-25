package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/spf13/cobra"
)

// Root-help sections for top-level groups.
var sections = map[string]string{
	"wp": "work", "project": "work", "time": "work", "version": "work", "membership": "work",
	"notification": "work", "meeting": "work", "query": "work", "whoami": "work",
	"user": "ref", "status": "ref", "type": "ref", "priority": "ref", "role": "ref",
	"api": "tools",
}

// addOps builds the command tree from the ops registry.
func addOps(root *cobra.Command, st *state) {
	byPath := map[string]*cobra.Command{"": root}

	gs := append([]*ops.Group(nil), ops.Groups()...)
	sort.SliceStable(gs, func(i, j int) bool {
		return strings.Count(gs[i].Name, " ") < strings.Count(gs[j].Name, " ")
	})
	for _, g := range gs {
		segs := strings.Fields(g.Name)
		parent := byPath[strings.Join(segs[:len(segs)-1], " ")]
		c := &cobra.Command{
			Use:     segs[len(segs)-1],
			Aliases: g.Aliases,
			Short:   g.Short,
			Long:    strings.TrimSpace(g.Short + "\n\n" + g.Long),
		}
		if len(segs) == 1 {
			c.GroupID = sections[g.Name]
		}
		parent.AddCommand(c)
		byPath[g.Name] = c
	}

	for _, op := range ops.All() {
		parent, ok := byPath[strings.Join(op.Cmd[:len(op.Cmd)-1], " ")]
		if !ok {
			panic(fmt.Sprintf("op %s: missing group %v", op.Tool, op.Cmd))
		}
		c := opCommand(op, st)
		if len(op.Cmd) == 1 {
			c.GroupID = sections[op.Cmd[0]]
		}
		parent.AddCommand(c)
	}
}

func opCommand(op *ops.Op, st *state) *cobra.Command {
	use := op.Cmd[len(op.Cmd)-1]
	required := 0
	pos := op.Positionals()
	rest := false
	for _, p := range pos {
		dots := ""
		if p.Rest {
			rest, dots = true, "..."
		}
		if p.Required {
			use += " <" + p.Flag() + dots + ">"
			required++
		} else {
			use += " [" + p.Flag() + dots + "]"
		}
	}
	argsCheck := cobra.RangeArgs(required, len(pos))
	if rest {
		argsCheck = cobra.MinimumNArgs(required)
	}

	c := &cobra.Command{
		Use:     use,
		Aliases: op.Aliases,
		Short:   op.Short,
		Long:    longHelp(op),
		Example: op.Example,
		Args:    argsCheck,
	}
	fs := c.Flags()
	for _, p := range op.Params {
		if p.Positional {
			continue
		}
		desc := p.Desc
		if p.Required {
			desc += " (required)"
		}
		switch p.Kind {
		case ops.String:
			def, _ := p.Default.(string)
			fs.StringP(p.Flag(), p.Short, def, desc)
		case ops.Int:
			def, _ := p.Default.(int)
			fs.IntP(p.Flag(), p.Short, def, desc)
		case ops.Bool:
			def, _ := p.Default.(bool)
			fs.BoolP(p.Flag(), p.Short, def, desc)
		case ops.Strings:
			fs.StringArrayP(p.Flag(), p.Short, nil, desc+" (repeatable or comma-separated)")
		}
		if p.File {
			fs.String(p.Flag()+"-file", "", fmt.Sprintf("Read --%s from a file ('-' = stdin)", p.Flag()))
		}
		if len(p.Enum) > 0 {
			enum := p.Enum
			_ = c.RegisterFlagCompletionFunc(p.Flag(), func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
				return enum, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}

	c.RunE = func(cmd *cobra.Command, args []string) error {
		a, err := collectArgs(cmd, op, args)
		if err != nil {
			return err
		}
		env, err := st.env()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		res, err := op.Execute(ctx, env, a)
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), st.output(), op, res)
	}
	return c
}

func longHelp(op *ops.Op) string {
	var b strings.Builder
	b.WriteString(op.Short)
	if op.Long != "" {
		b.WriteString("\n\n" + op.Long)
	}
	switch op.Safety {
	case ops.Write:
		b.WriteString("\n\nModifies data.")
	case ops.Destructive:
		b.WriteString("\n\nDestructive: cannot be undone.")
	}
	fmt.Fprintf(&b, "\nMCP tool: %s", op.Tool)
	return b.String()
}

func collectArgs(cmd *cobra.Command, op *ops.Op, args []string) (ops.Args, error) {
	a := ops.Args{}
	for i, p := range op.Positionals() {
		switch {
		case p.Rest && i < len(args):
			a[p.Name] = strings.Join(args[i:], " ")
		case i < len(args):
			a[p.Name] = args[i]
		}
	}
	fs := cmd.Flags()
	for _, p := range op.Params {
		if p.Positional {
			continue
		}
		name := p.Flag()
		if p.File && fs.Changed(name+"-file") {
			if fs.Changed(name) {
				return nil, fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name)
			}
			path, _ := fs.GetString(name + "-file")
			data, err := readPathOrStdin(path)
			if err != nil {
				return nil, err
			}
			a[p.Name] = string(data)
			continue
		}
		if !fs.Changed(name) {
			continue
		}
		switch p.Kind {
		case ops.String:
			a[p.Name], _ = fs.GetString(name)
		case ops.Int:
			a[p.Name], _ = fs.GetInt(name)
		case ops.Bool:
			a[p.Name], _ = fs.GetBool(name)
		case ops.Strings:
			a[p.Name], _ = fs.GetStringArray(name)
		}
	}
	return a, nil
}

// stdin is overridable in tests.
var stdin io.Reader = os.Stdin

func readPathOrStdin(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

func render(w io.Writer, format string, op *ops.Op, res any) error {
	switch format {
	case "json":
		if s, ok := res.(string); ok {
			_, err := fmt.Fprintln(w, s)
			return err
		}
		return ops.RenderJSON(w, res)
	case "text", "":
		return ops.RenderText(w, op, res)
	}
	return fmt.Errorf("unknown output format %q (text|json)", format)
}
