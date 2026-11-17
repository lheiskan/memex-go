package claudecode

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RegisterMCP points Claude Code's `memex` MCP server at command.
// An existing registration is repointed, not left alone.
func RegisterMCP(w io.Writer, command string, opts Options) {
	manual := fmt.Sprintf("claude mcp add --scope user memex -- %s", command)

	switch {
	case opts.SkipMCP:
		fmt.Fprintln(w, "MCP -> skipped (--skip-mcp)")
		return
	case opts.DryRun:
		fmt.Fprintf(w, "MCP -> would run: %s\n", manual)
		return
	}

	claude, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintln(w, "MCP -> claude CLI not found; register manually:")
		fmt.Fprintf(w, "       %s\n", manual)
		return
	}
	if out, err := exec.Command(claude, "mcp", "list").Output(); err == nil {
		switch got, found := registeredCommand(string(out), "memex"); {
		case found && got == command:
			fmt.Fprintln(w, "MCP -> already registered")
			return
		case found:
			fmt.Fprintf(w, "MCP -> repointing from %q\n", got)
			if err := exec.Command(claude, "mcp", "remove", "memex").Run(); err != nil {
				fmt.Fprintf(w, "MCP -> could not remove the old registration (%v); fix manually:\n", err)
				fmt.Fprintf(w, "       claude mcp remove memex && %s\n", manual)
				return
			}
		}
	}
	fmt.Fprintln(w, "MCP -> claude mcp add --scope user memex")
	args := append([]string{"mcp", "add", "--scope", "user", "memex", "--"}, strings.Fields(command)...)
	cmd := exec.Command(claude, args...)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(w, "MCP -> registration failed (%v); register manually:\n", err)
		fmt.Fprintf(w, "       %s\n", manual)
	}
}

// UnregisterMCP removes the `memex` MCP server.
func UnregisterMCP(w io.Writer, opts Options) {
	if opts.SkipMCP {
		fmt.Fprintln(w, "MCP -> skipped (--skip-mcp)")
		return
	}
	if opts.DryRun {
		fmt.Fprintln(w, "MCP -> would run: claude mcp remove memex")
		return
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintln(w, "MCP -> claude CLI not found; remove manually: claude mcp remove memex")
		return
	}
	if err := exec.Command(claude, "mcp", "remove", "memex").Run(); err != nil {
		fmt.Fprintf(w, "MCP -> not registered, or removal failed (%v)\n", err)
		return
	}
	fmt.Fprintln(w, "MCP -> removed")
}

// mcpRegistration reports the currently registered command, if any, and
// whether the `claude` CLI could be consulted at all.
func mcpRegistration() (command string, found, checkable bool) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		return "", false, false
	}
	out, err := exec.Command(claude, "mcp", "list").Output()
	if err != nil {
		return "", false, false
	}
	command, found = registeredCommand(string(out), "memex")
	return command, found, true
}

// registeredCommand finds a server's command in `claude mcp list` output,
// whose lines read "<name>: <command> - <status>".
//
// Parsed rather than matched so an existing registration can be compared and
// repointed. The format is the CLI's and could change, which is why a miss
// degrades to re-adding rather than to an error.
func registeredCommand(out, name string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), name+": ")
		if !ok {
			continue
		}
		if i := strings.LastIndex(rest, " - "); i >= 0 {
			rest = rest[:i]
		}
		return strings.TrimSpace(rest), true
	}
	return "", false
}
