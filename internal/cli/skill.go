package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carlosprados/openproject-cli/internal/skill"
	"github.com/spf13/cobra"
)

func skillCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "skill",
		Short: "Agent skill (SKILL.md) shipped with this binary",
		Long: `opcli ships with a skill document for coding agents (Claude Code and anything
else that reads a skills directory). It covers the commands, the JSON output,
names instead of ids, Markdown and images, and the OpenProject pitfalls, so an
agent drives the CLI correctly instead of guessing.

The document is embedded in this binary, so it always matches it:

  opcli skill install             into ~/.claude/skills/opcli/SKILL.md
  opcli skill install --repo      into ./.claude/skills/opcli/SKILL.md
  opcli skill show                print it
  opcli skill path                where install would write

Re-run 'opcli skill install --force' after upgrading the binary
('task install' does it).`,
	}

	show := &cobra.Command{
		Use:     "show",
		Short:   "Print the embedded skill document",
		Example: "  opcli skill show | less",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(skill.Content)
			return err
		},
	}

	path := &cobra.Command{
		Use:     "path",
		Short:   "Print where the skill would be installed",
		Example: "  opcli skill path\n  opcli skill path --repo",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := skillPath(cmd)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}

	install := &cobra.Command{
		Use:     "install",
		Short:   "Write the embedded skill where an agent will find it",
		Example: "  opcli skill install\n  opcli skill install --force      # after upgrading opcli\n  opcli skill install --repo       # only for the current repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := skillPath(cmd)
			if err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			if cur, err := os.ReadFile(p); err == nil {
				if bytes.Equal(cur, skill.Content) {
					fmt.Fprintln(cmd.OutOrStdout(), "Already up to date:", p)
					return nil
				}
				if !force {
					return fmt.Errorf("%s exists and differs from this version: use --force to overwrite", p)
				}
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(p, skill.Content, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Installed", p)
			return nil
		},
	}

	for _, s := range []*cobra.Command{install, path} {
		s.Flags().Bool("repo", false, "Use ./.claude/skills (this repository) instead of ~/.claude/skills")
	}
	install.Flags().Bool("force", false, "Overwrite an existing, different SKILL.md")
	c.AddCommand(install, show, path)
	return c
}

func skillPath(cmd *cobra.Command) (string, error) {
	base, err := os.UserHomeDir()
	if repo, _ := cmd.Flags().GetBool("repo"); repo {
		base, err = os.Getwd()
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".claude", "skills", skill.Name, "SKILL.md"), nil
}
