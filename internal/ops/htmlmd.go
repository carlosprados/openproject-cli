package ops

import (
	"html"
	"regexp"
	"strings"
)

// OpenProject's editor (CKEditor) stores Markdown with HTML where Markdown
// falls short: tables as one line of <figure><table>, <br> between list
// paragraphs, the odd <p>, <strong> or <a>. htmlToMarkdown turns the known
// tags back into Markdown outside code (fences and code spans are left
// alone); unknown tags are text ("<host>") and stay as typed.

var (
	fenceRe     = regexp.MustCompile("^\\s*(```|~~~)")
	codeSpanRe  = regexp.MustCompile("(`+)[^`]*?(`+)")
	brLineRe    = regexp.MustCompile(`(?i)^\s*<br\s*/?>\s*$`)
	detailsRe   = regexp.MustCompile(`(?i)^\s*</?details\b[^>]*>\s*$`)
	tableOpenRe = regexp.MustCompile(`(?i)<(figure|table)\b`)
	tableEndRe  = regexp.MustCompile(`(?i)</table>\s*(</figure>)?`)
	rowRe       = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	cellRe      = regexp.MustCompile(`(?is)<t([dh])\b[^>]*>(.*?)</t[dh]>`)
	anyTagRe    = regexp.MustCompile(`<[^>]+>`)
	spacesRe    = regexp.MustCompile(`\s+`)

	inlineRules = []struct {
		re   *regexp.Regexp
		repl func(m []string) string
	}{
		{regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`), func(m []string) string { return "`" + html.UnescapeString(m[1]) + "`" }},
		{regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`), func(m []string) string { return strings.Repeat("#", int(m[1][0]-'0')) + " " + m[2] }},
		{regexp.MustCompile(`(?is)<summary\b[^>]*>(.*?)</summary>`), func(m []string) string { return "**" + m[1] + "**" }},
		{regexp.MustCompile(`(?is)<(strong|b)\b[^>]*>(.*?)</(?:strong|b)>`), func(m []string) string { return "**" + m[2] + "**" }},
		{regexp.MustCompile(`(?is)<(em|i)\b[^>]*>(.*?)</(?:em|i)>`), func(m []string) string { return "*" + m[2] + "*" }},
		{regexp.MustCompile(`(?is)<(s|del|strike)\b[^>]*>(.*?)</(?:s|del|strike)>`), func(m []string) string { return "~~" + m[2] + "~~" }},
		{regexp.MustCompile(`(?is)<a\b[^>]*?\bhref="([^"]*)"[^>]*>(.*?)</a>`), func(m []string) string {
			href := html.UnescapeString(m[1])
			if strings.TrimSpace(m[2]) == "" {
				return "<" + href + ">"
			}
			return "[" + m[2] + "](" + href + ")"
		}},
		{regexp.MustCompile(`(?is)<img\b[^>]*>`), func(m []string) string {
			src := attr(m[0], "src")
			if src == "" {
				return ""
			}
			return "![" + attr(m[0], "alt") + "](" + src + ")"
		}},
		{regexp.MustCompile(`(?i)</p>\s*<p\b[^>]*>`), func([]string) string { return "\n\n" }},
		{regexp.MustCompile(`(?i)</?(p|span|u|sup|sub|mark|div|figure|tbody|thead)\b[^>]*>`), func([]string) string { return "" }},
	}
	entityRepl = strings.NewReplacer("&nbsp;", " ", "&quot;", `"`, "&#39;", "'", "&apos;", "'")
)

func attr(tag, name string) string {
	m := regexp.MustCompile(`(?i)\b` + name + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

func htmlToMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	// emit adds converted text; runs of blank lines (what removed tags
	// leave) become one. Lines inside fences go straight to out.
	emit := func(parts ...string) {
		for _, p := range parts {
			for _, l := range strings.Split(p, "\n") {
				if strings.TrimSpace(l) == "" {
					if len(out) > 0 && out[len(out)-1] == "" {
						continue
					}
					l = ""
				}
				out = append(out, l)
			}
		}
	}
	inFence := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if fenceRe.MatchString(l) {
			inFence = !inFence
			out = append(out, l)
			continue
		}
		if inFence {
			out = append(out, l)
			continue
		}
		if brLineRe.MatchString(l) || detailsRe.MatchString(l) {
			emit("")
			continue
		}
		if loc := tableOpenRe.FindStringIndex(l); loc != nil && !insideCodeSpan(l, loc[0]) {
			block := l
			for !tableEndRe.MatchString(block) && i+1 < len(lines) {
				i++
				block += "\n" + lines[i]
			}
			if end := tableEndRe.FindStringIndex(block); end != nil {
				indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
				before, table, after := block[:loc[0]], block[loc[0]:end[1]], block[end[1]:]
				if strings.TrimSpace(before) != "" {
					emit(inlineMarkdown(before))
				}
				emit("", tableMarkdown(table, indent), "")
				if strings.TrimSpace(after) != "" {
					emit(inlineMarkdown(after))
				}
				continue
			}
			emit(inlineMarkdown(block))
			continue
		}
		emit(inlineMarkdown(l))
	}
	return strings.Join(out, "\n")
}

func insideCodeSpan(l string, at int) bool {
	for _, m := range codeSpanRe.FindAllStringIndex(l, -1) {
		if at >= m[0] && at < m[1] {
			return true
		}
	}
	return false
}

// inlineMarkdown converts the known tags of one line outside code spans.
func inlineMarkdown(l string) string {
	indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
	var b strings.Builder
	last := 0
	for _, m := range codeSpanRe.FindAllStringIndex(l, -1) {
		b.WriteString(convertInline(l[last:m[0]], indent))
		b.WriteString(l[m[0]:m[1]])
		last = m[1]
	}
	b.WriteString(convertInline(l[last:], indent))
	return b.String()
}

var brRe = regexp.MustCompile(`(?i)<br\s*/?>`)

func convertInline(s, indent string) string {
	if !strings.Contains(s, "<") && !strings.Contains(s, "&") {
		return s
	}
	s = brRe.ReplaceAllString(s, "\n"+indent)
	for _, r := range inlineRules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string { return r.repl(r.re.FindStringSubmatch(m)) })
	}
	return entityRepl.Replace(s)
}

// tableMarkdown turns an HTML table into a GFM table; the first row is the
// header, as GFM needs one.
func tableMarkdown(table, indent string) string {
	var rows [][]string
	width := 0
	for _, r := range rowRe.FindAllStringSubmatch(table, -1) {
		var cells []string
		for _, c := range cellRe.FindAllStringSubmatch(r[1], -1) {
			cells = append(cells, cellMarkdown(c[2]))
		}
		width = max(width, len(cells))
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	line := func(cells []string) {
		b.WriteString(indent + "|")
		for i := range width {
			c := ""
			if i < len(cells) {
				c = cells[i]
			}
			b.WriteString(" " + c + " |")
		}
		b.WriteString("\n")
	}
	line(rows[0])
	b.WriteString(indent + "|" + strings.Repeat(" --- |", width) + "\n")
	for _, r := range rows[1:] {
		line(r)
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	cellHeadingRe = regexp.MustCompile(`(?is)<h[1-6]\b[^>]*>(.*?)</h[1-6]>`)
	boldRunRe     = regexp.MustCompile(`\*{4,}`)
)

func cellMarkdown(s string) string {
	s = cellHeadingRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = brRe.ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?i)</p>\s*<p\b[^>]*>`).ReplaceAllString(s, " ")
	s = convertInline(s, "")
	s = anyTagRe.ReplaceAllString(s, "")
	s = boldRunRe.ReplaceAllString(s, "**")
	s = strings.TrimSpace(spacesRe.ReplaceAllString(s, " "))
	return strings.ReplaceAll(s, "|", `\|`)
}
