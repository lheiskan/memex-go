// Package mcp exposes the memex storage operations over the Model Context
// Protocol via stdio.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcppkg "github.com/mark3labs/mcp-go/mcp"
	srv "github.com/mark3labs/mcp-go/server"

	"memex/card"
	"memex/graph"
	"memex/search"
	"memex/sync"
)

const defaultClientName = "mcp"

// ServeStdio starts the MCP server on stdin/stdout. Blocks until the client
// disconnects.
func ServeStdio(store *card.Store, home, version string) error {
	mcps := NewServer(store, home, version)
	// JS server runs autoFetch on connect — replicate by doing it here before
	// blocking on the transport.
	sync.AutoFetch(home)
	return srv.ServeStdio(mcps)
}

// NewServer builds the server with the upstream tool set registered.
// Exported so a test can drive it over an in-process transport.
func NewServer(store *card.Store, home, version string) *srv.MCPServer {
	mcps := srv.NewMCPServer("memex", version,
		srv.WithToolCapabilities(true),
		srv.WithLogging(),
	)
	registerTools(mcps, store, home)
	return mcps
}

func registerTools(s *srv.MCPServer, store *card.Store, home string) {
	// ---- Low-level tools ----

	s.AddTool(mcppkg.NewTool("memex_search",
		mcppkg.WithDescription("Keyword search over card titles and bodies. Case-insensitive AND over tokens: every token must appear in the same card, so a sentence or task summary matches nothing. Omit the query to list all cards."),
		mcppkg.WithString("query", mcppkg.Description("Search keywords, AND-matched: every token must appear in the same card, so keep it to 1-3 words. Omit to list all cards.")),
		mcppkg.WithNumber("limit", mcppkg.Description("Max results (default 10)")),
		mcppkg.WithBoolean("semantic", mcppkg.Description("Use embedding-based semantic search (NOT IMPLEMENTED in this build — keyword search is used)")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		query := getString(req, "query")
		limit := getInt(req, "limit", search.DefaultLimit)
		results, listAll, err := search.Run(store, query, search.Options{Limit: limit})
		if err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		return mcppkg.NewToolResultText(formatSearchResults(results, listAll)), nil
	})

	s.AddTool(mcppkg.NewTool("memex_read",
		mcppkg.WithDescription("Read one card in full by slug. Follow a search or an index entry with this — a slug list is not the memory."),
		mcppkg.WithString("slug", mcppkg.Required(), mcppkg.Description("Card slug (e.g. 'my-card-name')")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		slug := getString(req, "slug")
		raw, err := store.ReadCard(slug)
		if err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		return mcppkg.NewToolResultText(string(raw)), nil
	})

	s.AddTool(mcppkg.NewTool("memex_write",
		mcppkg.WithDescription("Write or overwrite a card. Frontmatter is authored for you: pass the body as content and the fields as arguments. One atomic insight per card, with [[wikilinks]] to related cards."),
		mcppkg.WithString("slug", mcppkg.Required(), mcppkg.Description("Card slug in kebab-case (e.g. 'my-insight')")),
		mcppkg.WithString("content", mcppkg.Required(), mcppkg.Description("Markdown body. May carry its own YAML frontmatter, in which case that frontmatter is preserved verbatim.")),
		mcppkg.WithString("title", mcppkg.Description("Card title (<=60 chars, noun phrase not full sentence). Required unless content carries frontmatter.")),
		mcppkg.WithString("category", mcppkg.Description("Card category (e.g. 'frontend', 'architecture', 'devops', 'bugfix')")),
		mcppkg.WithString("tags", mcppkg.Description("Comma-separated tags")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		slug := getString(req, "slug")
		existing, _ := store.ReadCard(slug) // nil when the card is new
		// Policy lives in card.Compose so this path and the CLI cannot drift
		// apart. DefaultSource is the calling client's own name, taken from
		// the initialize handshake.
		out, err := card.Compose([]byte(getString(req, "content")), existing, card.AuthorOptions{
			Title:         getString(req, "title"),
			Category:      getString(req, "category"),
			Tags:          getString(req, "tags"),
			Today:         todayISO(),
			DefaultSource: clientNameFromCtx(ctx),
		})
		if err != nil {
			return mcppkg.NewToolResultError(fmt.Sprintf("%s: pass title, or include frontmatter in content", err)), nil
		}
		if err := store.WriteCard(slug, out); err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		sync.AutoSync(home)
		return mcppkg.NewToolResultText(fmt.Sprintf("Card '%s' written.", slug)), nil
	})

	s.AddTool(mcppkg.NewTool("memex_links",
		mcppkg.WithDescription("Link graph stats, global or for one card. Prefer memex_organize for a full maintenance report."),
		mcppkg.WithString("slug", mcppkg.Description("Card slug. Omit for global stats.")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		slug := getString(req, "slug")
		m, err := graph.Build(store)
		if err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		if slug == "" {
			return mcppkg.NewToolResultText(graph.FormatStats(m.Stats())), nil
		}
		return mcppkg.NewToolResultText(graph.FormatCardLinks(slug, m.Outbound[slug], m.Inbound[slug])), nil
	})

	s.AddTool(mcppkg.NewTool("memex_archive",
		mcppkg.WithDescription("Move a card to the archive. Use for outdated or superseded cards."),
		mcppkg.WithString("slug", mcppkg.Required(), mcppkg.Description("Card slug to archive")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		slug := getString(req, "slug")
		if err := store.ArchiveCard(slug); err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		return mcppkg.NewToolResultText(fmt.Sprintf("Card '%s' archived.", slug)), nil
	})

	// ---- Tools that also sync (autoFetch before, autoSync after) ----

	s.AddTool(mcppkg.NewTool("memex_organize",
		mcppkg.WithDescription("Analyze the card network for maintenance. Returns link stats, orphans, hubs, unresolved conflicts, and recently modified cards paired with their neighbors for contradiction detection. Call this periodically to keep the knowledge graph healthy."),
		mcppkg.WithString("since", mcppkg.Description("Only check cards modified since this date (YYYY-MM-DD). Omit for full scan.")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		sync.AutoFetch(home)
		since := getString(req, "since")
		out, err := graph.Organize(store, since)
		if err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		sync.AutoSync(home)
		return mcppkg.NewToolResultText(out), nil
	})

	s.AddTool(mcppkg.NewTool("memex_pull",
		mcppkg.WithDescription("Pull latest cards from remote to get changes from other devices. If sync is not configured, DO NOT attempt to set it up yourself — tell the user to run these exact commands in their terminal: memex sync --init && memex sync on (this auto-creates a private 'memex-cards' repo on GitHub, requires gh CLI)."),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		if sync.ReadConfig(home).Remote == "" {
			return mcppkg.NewToolResultText("Sync not configured. DO NOT set this up yourself. Tell the user to run in their terminal: memex sync --init && memex sync on"), nil
		}
		r := sync.New(home).Pull()
		if !r.Success {
			return mcppkg.NewToolResultError(r.Message), nil
		}
		return mcppkg.NewToolResultText(r.Message), nil
	})

	s.AddTool(mcppkg.NewTool("memex_push",
		mcppkg.WithDescription("Push local cards to remote to sync to other devices. If sync is not configured, DO NOT attempt to set it up yourself — tell the user to run these exact commands in their terminal: memex sync --init && memex sync on (this auto-creates a private 'memex-cards' repo on GitHub, requires gh CLI)."),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		if sync.ReadConfig(home).Remote == "" {
			return mcppkg.NewToolResultText("Sync not configured. DO NOT set this up yourself. Tell the user to run in their terminal: memex sync --init && memex sync on"), nil
		}
		r := sync.New(home).Push()
		if !r.Success {
			return mcppkg.NewToolResultError(r.Message), nil
		}
		return mcppkg.NewToolResultText(r.Message), nil
	})
}

// clientNameFromCtx mirrors the JS server's clientInfo.name normalization
// (lowercase, whitespace → "-"). Falls back to "mcp" if unavailable.
func clientNameFromCtx(ctx context.Context) string {
	session := srv.ClientSessionFromContext(ctx)
	if session == nil {
		return defaultClientName
	}
	withInfo, ok := session.(srv.SessionWithClientInfo)
	if !ok {
		return defaultClientName
	}
	info := withInfo.GetClientInfo()
	name := strings.ToLower(strings.TrimSpace(info.Name))
	if name == "" {
		return defaultClientName
	}
	return strings.Join(strings.Fields(name), "-")
}

func todayISO() string { return time.Now().UTC().Format("2006-01-02") }

// --- request helpers ---

func getString(req mcppkg.CallToolRequest, key string) string {
	args := req.GetArguments()
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		// JSON numbers etc. — stringify.
		b, _ := json.Marshal(v)
		return strings.Trim(string(b), `"`)
	}
	return ""
}

func getInt(req mcppkg.CallToolRequest, key string, fallback int) int {
	args := req.GetArguments()
	if args == nil {
		return fallback
	}
	v, ok := args[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}

// --- formatting ---

func formatSearchResults(rs []search.Result, listAll bool) string {
	if len(rs) == 0 {
		return "No cards found."
	}
	if listAll {
		maxLen := 0
		for _, r := range rs {
			if len(r.Slug) > maxLen {
				maxLen = len(r.Slug)
			}
		}
		var b strings.Builder
		for _, r := range rs {
			fmt.Fprintf(&b, "%s%s%s\n", r.Slug, strings.Repeat(" ", maxLen+2-len(r.Slug)), r.Title)
		}
		return strings.TrimRight(b.String(), "\n")
	}
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		var b strings.Builder
		fmt.Fprintf(&b, "## %s\n%s\n%s", r.Slug, r.Title, r.FirstParagraph)
		if r.MatchLine != "" {
			fmt.Fprintf(&b, "\n> Matched line: %s", r.MatchLine)
		}
		if len(r.Links) > 0 {
			wikis := make([]string, len(r.Links))
			for i, l := range r.Links {
				wikis[i] = "[[" + l + "]]"
			}
			fmt.Fprintf(&b, "\nLinks: %s", strings.Join(wikis, ", "))
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n")
}
