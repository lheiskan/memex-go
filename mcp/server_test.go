package mcp

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcppkg "github.com/mark3labs/mcp-go/mcp"

	"memex/card"
)

// exposedTools is the tool surface this server means to expose: eight names,
// no more and no fewer. Asserted exactly rather than as a subset because the
// failure mode is growth — every added tool costs context in every session of
// every client, whether or not it is ever called. Two such tools were added
// and withdrawn inside a single session, which is why the bound is a test and
// not a convention.
//
// memex_recall and memex_retro are deliberately absent: they were the two
// tools that made judgment calls, which now belong to the skills.
var exposedTools = []string{
	"memex_archive",
	"memex_links",
	"memex_organize",
	"memex_pull",
	"memex_push",
	"memex_read",
	"memex_search",
	"memex_write",
}

// TestHandshakeAdvertisesUpstreamTools drives the real server over mcp-go's
// in-process transport, which exercises the same initialize / tools/list path
// a stdio client would.
func TestHandshakeAdvertisesUpstreamTools(t *testing.T) {
	home := t.TempDir()
	store := card.NewStore(filepath.Join(home, "cards"), filepath.Join(home, "archive"), false)

	c, err := mcpclient.NewInProcessClient(NewServer(store, home, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var init mcppkg.InitializeRequest
	init.Params.ProtocolVersion = mcppkg.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcppkg.Implementation{Name: "Test Client", Version: "1"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}

	tools, err := c.ListTools(ctx, mcppkg.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	if len(got) != len(exposedTools) {
		t.Fatalf("got %d tools, want %d:\n got:  %v\n want: %v",
			len(got), len(exposedTools), got, exposedTools)
	}
	for i := range got {
		if got[i] != exposedTools[i] {
			t.Errorf("tool[%d]: got %s, want %s", i, got[i], exposedTools[i])
		}
	}
}

// memex_write is the only remaining consumer of mcp-go's typed clientInfo
// accessor, so it carries the source-stamping claim.
//
// What this test can and cannot show: mcp-go's in-process transport hands the
// handler a session that does not satisfy SessionWithClientInfo, so clientInfo
// never arrives and the source falls back to defaultClientName. That fallback
// exists for exactly this case — a nil session, a test transport, or a client
// that does not implement the interface — so it is what gets asserted here,
// and the real stamping has to be verified over a real transport:
//
//	{ printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"My Test Client","version":"1"}}}'
//	  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
//	  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"memex_write","arguments":{"slug":"stamped","content":"An insight.","title":"A Stamped Card"}}}'
//	  sleep 0.4
//	} | memex mcp
//
// Confirmed against a scratch MEMEX_HOME on 2026-11-17: source: my-test-client.
func TestWriteAuthorsFrontmatter(t *testing.T) {
	home := t.TempDir()
	store := card.NewStore(filepath.Join(home, "cards"), filepath.Join(home, "archive"), false)

	c, err := mcpclient.NewInProcessClient(NewServer(store, home, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var init mcppkg.InitializeRequest
	init.Params.ProtocolVersion = mcppkg.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcppkg.Implementation{Name: "My Test Client", Version: "1"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}

	var call mcppkg.CallToolRequest
	call.Params.Name = "memex_write"
	call.Params.Arguments = map[string]any{
		"slug":     "stamped",
		"content":  "An insight worth keeping.",
		"title":    "A Stamped Card",
		"category": "testing",
	}
	res, err := c.CallTool(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("memex_write returned an error: %+v", res.Content)
	}

	raw, err := store.ReadCard("stamped")
	if err != nil {
		t.Fatal(err)
	}
	fm, body := card.ParseFrontmatter(raw)
	if got := fm.GetString("source"); got != defaultClientName {
		t.Errorf("source = %q, want the %q fallback", got, defaultClientName)
	}
	if got := fm.GetString("title"); got != "A Stamped Card" {
		t.Errorf("title = %q, want %q", got, "A Stamped Card")
	}
	if got := fm.GetString("category"); got != "testing" {
		t.Errorf("category = %q, want %q", got, "testing")
	}
	for _, f := range []string{"created", "modified"} {
		if fm.GetString(f) == "" {
			t.Errorf("%s was not stamped", f)
		}
	}
	// The agent sent prose, not file layout; Compose owns the blank line.
	if string(body) != "\nAn insight worth keeping.\n" {
		t.Errorf("body = %q, want %q", body, "\nAn insight worth keeping.\n")
	}
}
