package cli

import (
	"io"
	"strings"
	"testing"
)

func run(t *testing.T, input string, terminal bool, args ...string) error {
	t.Helper()
	oldStdin, oldTerm := stdin, isTerminal
	t.Cleanup(func() { stdin, isTerminal = oldStdin, oldTerm })
	stdin = strings.NewReader(input)
	isTerminal = func() bool { return terminal }
	root := NewRoot(BuildInfo{})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

// Deletes never run unattended without --yes, and a refused prompt stops
// before any config or network access (the config here does not exist).
func TestDestructiveNeedsConfirmation(t *testing.T) {
	noConfig := []string{"--config", "/nonexistent/opcli.yaml"}
	if err := run(t, "", false, append([]string{"wp", "delete", "42"}, noConfig...)...); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("no terminal, no --yes: got %v", err)
	}
	if err := run(t, "n\n", true, append([]string{"wp", "delete", "42"}, noConfig...)...); err == nil || err.Error() != "cancelled" {
		t.Errorf("answered no: got %v", err)
	}
	// Confirmed: it gets past the prompt and fails later, on the config.
	for _, input := range []string{"y\n", ""} {
		args := append([]string{"wp", "delete", "42"}, noConfig...)
		if input == "" {
			args = append(args, "--yes")
		}
		if err := run(t, input, input != "", args...); err == nil || strings.Contains(err.Error(), "--yes") || err.Error() == "cancelled" {
			t.Errorf("confirmed (%q): got %v", input, err)
		}
	}
	// api request asks only for DELETE.
	if err := run(t, "", false, append([]string{"api", "request", "GET", "projects"}, noConfig...)...); err != nil && strings.Contains(err.Error(), "--yes") {
		t.Errorf("GET must not ask: %v", err)
	}
	if err := run(t, "", false, append([]string{"api", "request", "DELETE", "projects/1"}, noConfig...)...); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("DELETE must ask: %v", err)
	}
}
