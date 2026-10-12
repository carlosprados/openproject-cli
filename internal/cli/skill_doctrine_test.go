package cli

// Doctrine: the CLI and internal/skill/SKILL.md are twins. Agents read the
// skill instead of --help, so a command or flag the skill does not mention is,
// for them, missing; and a command the skill shows must run as written. These
// tests fail whenever one side changes without the other.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/carlosprados/openproject-cli/internal/skill"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// notForAgents lists what the skill deliberately leaves out, with the reason.
// Keep it short: anything an agent could use belongs in the skill.
var notForAgents = map[string]string{
	"skill install": "installs this very document; humans and the Taskfile run it",
	"skill path":    "installation detail",
	"docs":          "generates docs/commands.md for the repository",
	"--force":       "skill install only",
	"--repo":        "skill install/path only",
}

func skillText() string { return string(skill.Content) }

// leafCommands returns every runnable command by its path without "opcli ".
func leafCommands(c *cobra.Command) map[string]*cobra.Command {
	out := map[string]*cobra.Command{}
	for _, sub := range c.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		if sub.Runnable() {
			out[strings.TrimPrefix(sub.CommandPath(), "opcli ")] = sub
		}
		for p, cc := range leafCommands(sub) {
			out[p] = cc
		}
	}
	return out
}

func TestSkillMentionsEveryCommand(t *testing.T) {
	text := skillText()
	for path := range leafCommands(NewRoot(BuildInfo{})) {
		if _, ok := notForAgents[path]; ok {
			continue
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(path) + `\b`).MatchString(text) {
			t.Errorf("command %q is not in SKILL.md: document it or add it to notForAgents", path)
		}
	}
}

func TestSkillMentionsEveryFlag(t *testing.T) {
	text := skillText()
	seen := map[string]bool{}
	for path, c := range leafCommands(NewRoot(BuildInfo{})) {
		if _, ok := notForAgents[path]; ok {
			continue
		}
		c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
			name := "--" + f.Name
			if f.Hidden || f.Name == "help" || seen[name] {
				return
			}
			seen[name] = true
			if _, ok := notForAgents[name]; ok {
				return
			}
			// "--description" must not be satisfied by "--description-file".
			if !regexp.MustCompile(regexp.QuoteMeta(name) + `([^\w-]|$)`).MatchString(text) {
				t.Errorf("flag %s (%s) is not in SKILL.md: document it or add it to notForAgents", name, path)
			}
		})
	}
}

func TestSkillCommandsParse(t *testing.T) {
	root := NewRoot(BuildInfo{})
	root.InitDefaultHelpCmd() // "opcli help filters" in the skill
	lines := skillCommandLines(skillText())
	if len(lines) < 60 {
		t.Fatalf("only %d commands extracted from SKILL.md: extractor broken?", len(lines))
	}
	for _, line := range lines {
		if err := checkLine(root, line); err != nil {
			t.Errorf("%q: %v", line, err)
		}
	}
	// The checker itself must reject what is wrong.
	for _, bad := range []string{
		"opcli wp update 42 --labels x",
		"opcli wp frobnicate 42",
		"opcli wp get",
	} {
		if checkLine(root, bad) == nil {
			t.Errorf("checkLine accepted %q", bad)
		}
	}
}

// checkLine resolves an "opcli …" line against the real command tree and
// parses its flags and arguments without running it.
func checkLine(root *cobra.Command, line string) error {
	args := shellWords(line)[1:]
	c, rest, err := root.Find(args)
	if err != nil {
		return err
	}
	if c == root || !c.Runnable() {
		return fmt.Errorf("no such command")
	}
	defer c.Flags().VisitAll(func(f *pflag.Flag) { // reset for the next line
		if f.Changed {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		}
	})
	c.InitDefaultHelpFlag()
	if err := c.ParseFlags(rest); err != nil {
		return err
	}
	return c.ValidateArgs(c.Flags().Args())
}

// skillCommandLines extracts every "opcli …" invocation from the sh blocks,
// cutting comments, pipes and $( ) wrappers.
func skillCommandLines(text string) []string {
	var out []string
	inBlock := false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "```sh"):
			inBlock = true
			continue
		case strings.HasPrefix(l, "```"):
			inBlock = false
			continue
		}
		if !inBlock {
			continue
		}
		for _, seg := range strings.Split(l, " · ") {
			i := strings.Index(seg, "opcli ")
			if i < 0 {
				continue
			}
			cmd := strings.TrimSpace(cutUnquoted(seg[i:], "#|)"))
			if strings.Contains(cmd, "--help") || strings.Contains(cmd, "<") {
				continue
			}
			out = append(out, cmd)
		}
	}
	return out
}

// cutUnquoted cuts s at the first of seps found outside quotes.
func cutUnquoted(s, seps string) string {
	var q rune
	for i, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			}
		case r == '"' || r == '\'':
			q = r
		case strings.ContainsRune(seps, r):
			return s[:i]
		}
	}
	return s
}

// shellWords splits like a POSIX shell for the simple quoting the skill uses.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	var q rune
	inWord := false
	for _, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			q, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}
