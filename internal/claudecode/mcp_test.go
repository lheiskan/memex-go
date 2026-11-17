package claudecode

import "testing"

// The real output of `claude mcp list`, captured 2026-11-17. Parsed rather
// than grepped so an existing registration can be compared against what we
// would write and repointed if it differs. A bare PATH-resolved "memex mcp"
// is the case that matters: it looks registered and resolves at spawn time.
const mcpListOutput = `Checking MCP server health…

claude.ai Claude Docs: https://api.anthropic.com/v1/pages/mcp - ✔ Connected
claude.ai Gmail: https://gmailmcp.googleapis.com/mcp/v1 - ✔ Connected
manic: manic mcp - ✔ Connected
memex: memex mcp - ✔ Connected
`

func TestRegisteredCommand(t *testing.T) {
	tests := []struct {
		name      string
		out       string
		server    string
		want      string
		wantFound bool
	}{
		{"bare path-resolved command", mcpListOutput, "memex", "memex mcp", true},
		{"a neighbour is not confused for us", mcpListOutput, "manic", "manic mcp", true},
		{"absent server", mcpListOutput, "nope", "", false},
		{
			"absolute path with spaces survives",
			"memex: /Users/a b/go/bin/memex mcp - ✔ Connected\n",
			"memex", "/Users/a b/go/bin/memex mcp", true,
		},
		{
			"a failing server still yields its command",
			"memex: /bin/mcc mcp - ✘ Failed to connect\n",
			"memex", "/bin/mcc mcp", true,
		},
		{
			// "memex-notes" must not satisfy a lookup for "memex".
			"prefix of a longer name is not a match",
			"memex-notes: other mcp - ✔ Connected\n",
			"memex", "", false,
		},
		{"empty output", "", "memex", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := registeredCommand(tt.out, tt.server)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.want {
				t.Errorf("command = %q, want %q", got, tt.want)
			}
		})
	}
}
