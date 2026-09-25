package ops

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Keys shown first, in this order, when rendering a single record.
var recordOrder = []string{
	"id", "displayId", "identifier", "subject", "name", "title", "_type",
	"type", "status", "priority", "project", "parent", "assignee",
	"responsible", "author", "user", "version", "startDate", "dueDate",
	"spentOn", "hours", "estimatedTime", "remainingTime", "spentTime",
	"percentageDone", "active", "public", "url", "createdAt", "updatedAt",
}

// Long text keys rendered last, as blocks.
var blockKeys = map[string]bool{"description": true, "comment": true, "notes": true, "statusExplanation": true, "summary": true, "text": true}

// RenderText writes v for humans: List → table, map → record, other → JSON.
func RenderText(w io.Writer, op *Op, v any) error {
	if op != nil && op.Text != nil {
		return op.Text(w, v)
	}
	switch x := v.(type) {
	case *List:
		var cols []Column
		if op != nil {
			cols = op.Columns
		}
		return renderTable(w, x, cols)
	case map[string]any:
		return renderRecord(w, x)
	case *Message:
		_, err := fmt.Fprintln(w, x.Message)
		return err
	case string:
		_, err := fmt.Fprintln(w, x)
		return err
	default:
		return RenderJSON(w, v)
	}
}

// RenderJSON writes v as indented JSON.
func RenderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func renderTable(w io.Writer, l *List, cols []Column) error {
	if len(l.Items) == 0 {
		_, err := fmt.Fprintln(w, "No results.")
		return err
	}
	if len(cols) == 0 {
		cols = autoColumns(l.Items)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	hdr := make([]string, len(cols))
	for i, c := range cols {
		hdr[i] = c.Header
	}
	fmt.Fprintln(tw, strings.Join(hdr, "\t"))
	for _, it := range l.Items {
		row := make([]string, len(cols))
		for i, c := range cols {
			val := it[c.Key]
			if c.Fmt != nil {
				row[i] = c.Fmt(val)
			} else {
				row[i] = hal.String(val)
			}
			row[i] = truncate(strings.ReplaceAll(row[i], "\n", " "), 60)
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if l.Total > l.Count {
		_, err := fmt.Fprintf(w, "\nShowing %d of %d (use --max / --page for more).\n", l.Count, l.Total)
		return err
	}
	return nil
}

func autoColumns(items []map[string]any) []Column {
	var cols []Column
	for _, k := range []string{"id", "name", "subject", "title", "status", "project", "type"} {
		if _, ok := items[0][k]; ok {
			cols = append(cols, Column{Header: strings.ToUpper(k), Key: k})
		}
	}
	if len(cols) == 0 {
		keys := sortedKeys(items[0])
		for _, k := range keys[:min(len(keys), 6)] {
			cols = append(cols, Column{Header: strings.ToUpper(k), Key: k})
		}
	}
	return cols
}

func renderRecord(w io.Writer, m map[string]any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	seen := map[string]bool{}
	var blocks []string
	emit := func(k string) {
		v, ok := m[k]
		if !ok || seen[k] {
			return
		}
		seen[k] = true
		if blockKeys[k] {
			blocks = append(blocks, k)
			return
		}
		switch v.(type) {
		case []map[string]any, map[string]any:
			blocks = append(blocks, k)
			return
		}
		if v == nil || v == "" {
			return
		}
		s := hal.String(v)
		if strings.HasSuffix(k, "Time") && strings.HasPrefix(s, "P") {
			s = hal.HoursString(v)
		}
		fmt.Fprintf(tw, "%s:\t%s\n", k, s)
	}
	for _, k := range recordOrder {
		emit(k)
	}
	for _, k := range sortedKeys(m) {
		if strings.HasSuffix(k, "Id") || strings.HasSuffix(k, "Ids") {
			seen[k] = true // ids are noise next to their titles in text mode
			continue
		}
		emit(k)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, k := range blocks {
		switch v := m[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				fmt.Fprintf(w, "\n%s:\n%s\n", k, indent(v))
			}
		case []map[string]any:
			fmt.Fprintf(w, "\n%s (%d):\n", k, len(v))
			for _, it := range v {
				fmt.Fprintf(w, "  - %s\n", oneLine(it))
			}
		case map[string]any:
			fmt.Fprintf(w, "\n%s:\n  %s\n", k, oneLine(v))
		}
	}
	return nil
}

func oneLine(m map[string]any) string {
	var parts []string
	for _, k := range []string{"id", "type", "name", "subject", "title", "fileName", "status", "to", "from"} {
		if v, ok := m[k]; ok && v != nil && v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", k, hal.String(v)))
		}
	}
	if len(parts) == 0 {
		b, _ := json.Marshal(m)
		return string(b)
	}
	return strings.Join(parts, " ")
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Common column formatters.
var (
	hoursFmt = func(v any) string { return hal.HoursString(v) }
	pctFmt   = func(v any) string {
		if v == nil {
			return ""
		}
		return hal.String(v) + "%"
	}
)

// renderAllowed prints the field → allowed values map of `wp allowed`.
func renderAllowed(w io.Writer, v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return RenderJSON(w, v)
	}
	for _, k := range sortedKeys(m) {
		vals, _ := m[k].([]map[string]any)
		parts := make([]string, len(vals))
		for i, it := range vals {
			parts[i] = fmt.Sprintf("%s (%s)", hal.String(it["name"]), hal.String(it["id"]))
		}
		if len(parts) == 0 {
			parts = []string{"-"}
		}
		if _, err := fmt.Fprintf(w, "%s: %s\n", k, strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	return nil
}
