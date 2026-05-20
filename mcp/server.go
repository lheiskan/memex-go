// Package mcp exposes the memex tools over the Model Context Protocol via stdio.
//
// Replicates the upstream JS server (10 tools), with one structural improvement:
// client identification uses mcp-go's SessionWithClientInfo accessor instead of
// monkey-patching the transport's onmessage. Tool semantics — frontmatter
// stamping, autoFetch/autoSync hooks, descriptions — match upstream exactly.
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
	mcps := srv.NewMCPServer("memex", version,
		srv.WithToolCapabilities(true),
		srv.WithLogging(),
	)
	registerTools(mcps, store, home)
	// JS server runs autoFetch on connect — replicate by doing it here before
	// blocking on the transport.
	sync.AutoFetch(home)
	return srv.ServeStdio(mcps)
}

func registerTools(s *srv.MCPServer, store *card.Store, home string) {
	// ---- Low-level tools ----

	s.AddTool(mcppkg.NewTool("memex_search",
		mcppkg.WithDescription("Low-level search. Prefer memex_recall for task-start workflows."),
		mcppkg.WithString("query", mcppkg.Description("Search keyword. Omit to list all cards.")),
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
		mcppkg.WithDescription("Low-level read. Use after memex_recall to drill into specific cards."),
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
		mcppkg.WithDescription("Low-level write. Prefer memex_retro for task-end workflows (handles frontmatter and sync automatically)."),
		mcppkg.WithString("slug", mcppkg.Required(), mcppkg.Description("Card slug in kebab-case (e.g. 'my-insight')")),
		mcppkg.WithString("content", mcppkg.Required(), mcppkg.Description("Full card content: YAML frontmatter + markdown body. Title must be ≤60 chars, noun phrase not full sentence.")),
		mcppkg.WithString("category", mcppkg.Description("Card category (e.g. 'frontend', 'architecture', 'devops', 'bugfix')")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		slug := getString(req, "slug")
		content := getString(req, "content")
		category := getString(req, "category")
		fm, body := card.ParseFrontmatter([]byte(content))
		if _, ok := fm.Get("source"); !ok {
			fm.Set("source", clientNameFromCtx(ctx))
		}
		if category != "" {
			if _, ok := fm.Get("category"); !ok {
				fm.Set("category", category)
			}
		}
		// Required fields check matches the CLI write path.
		for _, f := range []string{"title", "created", "source"} {
			if _, ok := fm.Get(f); !ok {
				return mcppkg.NewToolResultError(fmt.Sprintf("missing required field: %s", f)), nil
			}
		}
		fm.Set("modified", todayISO())
		out := card.StringifyFrontmatter(body, fm)
		if err := store.WriteCard(slug, out); err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		return mcppkg.NewToolResultText(fmt.Sprintf("Card '%s' written successfully.", slug)), nil
	})

	s.AddTool(mcppkg.NewTool("memex_links",
		mcppkg.WithDescription("Low-level link stats. Prefer memex_organize for maintenance workflows."),
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

	// ---- High-level tools (with autoFetch/autoSync hooks) ----

	s.AddTool(mcppkg.NewTool("memex_recall",
		mcppkg.WithDescription("IMPORTANT: You MUST call this at the START of every new task or conversation, BEFORE doing any work. This retrieves your persistent memory — knowledge cards from previous sessions with [[bidirectional links]]. Returns the keyword index (if exists) or card list. Optionally search by query. Without calling this first, you will miss context from prior sessions and repeat past mistakes."),
		mcppkg.WithString("query", mcppkg.Description("Optional search query to find specific cards")),
		mcppkg.WithString("category", mcppkg.Description("Filter by frontmatter category")),
		mcppkg.WithString("tag", mcppkg.Description("Filter by frontmatter tag")),
		mcppkg.WithString("author", mcppkg.Description("Filter by frontmatter author/source")),
		mcppkg.WithString("since", mcppkg.Description("Only cards created/modified after this date (YYYY-MM-DD)")),
		mcppkg.WithString("before", mcppkg.Description("Only cards created/modified before this date (YYYY-MM-DD)")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		sync.AutoFetch(home)
		query := getString(req, "query")
		filter := search.Filter{
			Category: getString(req, "category"),
			Tag:      getString(req, "tag"),
			Author:   getString(req, "author"),
			Since:    getString(req, "since"),
			Before:   getString(req, "before"),
		}
		if query != "" {
			results, listAll, err := search.Run(store, query, search.Options{Limit: search.DefaultLimit, Filter: filter})
			if err != nil {
				return mcppkg.NewToolResultError(err.Error()), nil
			}
			return mcppkg.NewToolResultText(formatSearchResults(results, listAll)), nil
		}
		// No query — try index card first, fall back to listing.
		if filter.Empty() {
			if raw, err := store.ReadCard("index"); err == nil {
				return mcppkg.NewToolResultText(string(raw)), nil
			}
		}
		results, listAll, err := search.Run(store, "", search.Options{Filter: filter})
		if err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		return mcppkg.NewToolResultText(formatSearchResults(results, listAll)), nil
	})

	s.AddTool(mcppkg.NewTool("memex_retro",
		mcppkg.WithDescription("IMPORTANT: Call this at the END of every task to save what you learned. Write one atomic insight per card with [[wikilinks]] to related cards. Only save non-obvious learnings — things that would be useful in future sessions (architecture decisions, gotchas, patterns discovered, bug root causes). Handles frontmatter, source tagging, and cross-device sync automatically."),
		mcppkg.WithString("slug", mcppkg.Required(), mcppkg.Description("Card slug in kebab-case")),
		mcppkg.WithString("title", mcppkg.Required(), mcppkg.Description("Card title (keep short, ≤60 chars, noun phrase not full sentence)")),
		mcppkg.WithString("body", mcppkg.Required(), mcppkg.Description("Card body in markdown with [[wikilinks]]")),
		mcppkg.WithString("category", mcppkg.Description("Category (e.g. frontend, architecture, devops, bugfix)")),
	), func(ctx context.Context, req mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error) {
		sync.AutoFetch(home)
		slug := getString(req, "slug")
		title := getString(req, "title")
		body := getString(req, "body")
		category := getString(req, "category")
		today := todayISO()
		fm := card.Frontmatter{}
		fm.Set("title", title)
		fm.Set("created", today)
		fm.Set("source", clientNameFromCtx(ctx))
		if category != "" {
			fm.Set("category", category)
		}
		fm.Set("modified", today)
		out := card.StringifyFrontmatter([]byte(body), fm)
		if err := store.WriteCard(slug, out); err != nil {
			return mcppkg.NewToolResultError(err.Error()), nil
		}
		sync.AutoSync(home)
		tip := ""
		if sync.ReadConfig(home).Remote == "" {
			tip = "\n\nTip: To sync cards across devices, run: memex sync --init && memex sync on"
		}
		return mcppkg.NewToolResultText(fmt.Sprintf("Card '%s' saved.%s", slug, tip)), nil
	})

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
			fmt.Fprintf(&b, "\n> 匹配行: %s", r.MatchLine)
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
