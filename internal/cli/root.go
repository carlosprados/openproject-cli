// Package cli builds the opcli Cobra command tree. Domain commands are
// generated from internal/ops; this package adds setup commands (config,
// mcp, guide, docs), help topics and output handling.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/carlosprados/openproject-cli/internal/client"
	"github.com/carlosprados/openproject-cli/internal/config"
	"github.com/carlosprados/openproject-cli/internal/guide"
	"github.com/carlosprados/openproject-cli/internal/ops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// BuildInfo is injected by main (GoReleaser ldflags).
type BuildInfo struct {
	Version, Commit, Date string
}

// state is shared by all commands of one invocation.
type state struct {
	v          *viper.Viper
	configFile string
	build      BuildInfo

	once sync.Once
	cfg  *config.Config
	err  error
}

func (s *state) config() (*config.Config, error) {
	s.once.Do(func() { s.cfg, s.err = config.Load(s.v, s.configFile) })
	return s.cfg, s.err
}

func (s *state) env() (*ops.Env, error) {
	cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	c := client.New(cfg.URL, cfg.APIKey)
	c.UserAgent = "opcli/" + s.build.Version
	env := ops.NewEnv(c, cfg.Project)
	env.DefaultType = cfg.DefaultType
	env.Language = cfg.Language
	return env, nil
}

func (s *state) output() string { return strings.ToLower(s.v.GetString("output")) }

const rootLong = `opcli — OpenProject from the command line, for humans and AI agents.

Everything is self-documented: every command has --help with examples.
Start here:

  opcli config init --url https://op.example.com --api-key <key>
  opcli whoami
  opcli wp list --assignee me --project all
  opcli guide                  # concepts, conventions and recipes

References are resolved by name (projects, users, statuses, types...), so
you rarely need ids. Output is a table by default and JSON with -o json.

Every command is also an MCP tool: run 'opcli mcp serve' and connect your
AI client ('opcli mcp config' prints the snippet). 'opcli api' reaches any
endpoint of the API that has no dedicated command.`

// NewRoot builds the root command.
func NewRoot(b BuildInfo) *cobra.Command {
	st := &state{v: viper.New(), build: b}
	root := &cobra.Command{
		Use:           "opcli",
		Short:         "OpenProject CLI and MCP server",
		Long:          rootLong,
		Version:       b.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate(fmt.Sprintf("opcli %s (commit %s, built %s)\n", b.Version, b.Commit, b.Date))

	pf := root.PersistentFlags()
	pf.StringVar(&st.configFile, "config", "", "Config file (default ~/.openproject.yaml)")
	pf.String("url", "", "OpenProject base URL (env OPENPROJECT_URL)")
	pf.String("api-key", "", "API key (env OPENPROJECT_API_KEY)")
	pf.StringP("output", "o", "text", "Output format: text or json")
	_ = st.v.BindPFlag("url", pf.Lookup("url"))
	_ = st.v.BindPFlag("api_key", pf.Lookup("api-key"))
	_ = st.v.BindPFlag("output", pf.Lookup("output"))
	_ = root.RegisterFlagCompletionFunc("output", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp
	})

	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work commands:"},
		&cobra.Group{ID: "ref", Title: "Reference data:"},
		&cobra.Group{ID: "tools", Title: "Setup, AI and raw API:"},
	)
	addOps(root, st)
	for _, c := range []*cobra.Command{configCmd(st), mcpCmd(st), guideCmd(), docsCmd()} {
		c.GroupID = "tools"
		root.AddCommand(c)
	}
	root.SetHelpCommandGroupID("tools")
	root.SetCompletionCommandGroupID("tools")
	for _, t := range helpTopics {
		root.AddCommand(t)
	}
	return root
}

// Execute runs opcli and returns the process exit code.
func Execute(b BuildInfo) int {
	root := NewRoot(b)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			fmt.Fprintln(os.Stderr, "Hint: check the API key (opcli config show / opcli help auth).")
		}
		return 1
	}
	return 0
}

func configCmd(st *state) *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Configure the server URL, API key and default project",
		Long: `Manage ~/.openproject.yaml. Precedence: flags > environment
(OPENPROJECT_URL, OPENPROJECT_API_KEY, OPENPROJECT_PROJECT) > .env in the
current directory > config file. See 'opcli help auth'.`,
	}

	var url, key, project, defaultType, language, file string
	var noVerify bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Write the config file after verifying the credentials",
		Example: `  opcli config init --url https://op.example.com --api-key 0123abcd...
  opcli config init --url http://localhost:8080 --api-key $KEY --project demo-project`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if url == "" || key == "" {
				return errors.New("--url and --api-key are required")
			}
			cfg := &config.Config{URL: strings.TrimRight(url, "/"), APIKey: key, Project: project, DefaultType: defaultType, Language: language}
			if !noVerify {
				me, err := client.New(cfg.URL, cfg.APIKey).Get(context.Background(), "/users/me", nil)
				if err != nil {
					return fmt.Errorf("credentials rejected: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Authenticated as %v (%v)\n", me["name"], me["login"])
			}
			if file == "" {
				file = st.configFile
			}
			if file == "" {
				file = config.DefaultFile()
			}
			if err := cfg.Save(file); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s (mode 0600)\n", file)
			return nil
		},
	}
	initCmd.Flags().StringVar(&url, "url", "", "OpenProject base URL, e.g. https://op.example.com")
	initCmd.Flags().StringVar(&key, "api-key", "", "API key (My account → Access tokens → API)")
	initCmd.Flags().StringVar(&project, "project", "", "Default project (id or identifier) for commands that need one")
	initCmd.Flags().StringVar(&defaultType, "default-type", "", "Type of new work packages when --type is omitted (default: the project's default type)")
	initCmd.Flags().StringVar(&language, "language", "", "Language of text opcli writes to the server, such as audit comments: en (default) or es")
	initCmd.Flags().StringVar(&file, "file", "", "Where to write (default ~/.openproject.yaml)")
	initCmd.Flags().BoolVar(&noVerify, "no-verify", false, "Do not check the credentials against the server")

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the effective configuration (API key masked)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := st.config()
			if err != nil {
				return err
			}
			out := map[string]any{"file": cfg.File, "url": cfg.URL, "api_key": cfg.MaskedKey(), "project": cfg.Project, "default_type": cfg.DefaultType, "language": cfg.Language}
			if cfg.APIKey == "" {
				out["api_key"] = ""
			}
			if st.output() == "json" {
				return ops.RenderJSON(cmd.OutOrStdout(), out)
			}
			for _, k := range []string{"file", "url", "api_key", "project", "default_type", "language"} {
				fmt.Fprintf(cmd.OutOrStdout(), "%-13s %v\n", k+":", out[k])
			}
			return nil
		},
	}
	c.AddCommand(initCmd, showCmd)
	return c
}

func guideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "Print the usage guide: concepts, conventions and recipes",
		Long:  "Print the usage guide (Markdown). The same text is served to AI agents as the MCP resource openproject://guide.",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), guide.Text)
		},
	}
}
