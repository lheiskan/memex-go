// memex — Zettelkasten agent memory CLI (Go reimplementation).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"memex/card"
	"memex/graph"
	"memex/mcp"
	"memex/search"
	"memex/sync"
)

const Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "search":
		err = cmdSearch(args)
	case "read":
		err = cmdRead(args)
	case "write":
		err = cmdWrite(args)
	case "links":
		err = cmdLinks(args)
	case "backlinks":
		err = cmdBacklinks(args)
	case "archive":
		err = cmdArchive(args)
	case "organize":
		err = cmdOrganize(args)
	case "sync":
		err = cmdSync(args)
	case "mcp":
		err = cmdMCP(args)
	case "version", "--version", "-v":
		fmt.Println(Version)
	case "help", "--help", "-h":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "memex — Zettelkasten agent memory CLI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  search [query]    Full-text search cards (body only), or list all if no query")
	fmt.Fprintln(w, "  read <slug>       Read a card's full content")
	fmt.Fprintln(w, "  write <slug>      Write a card (content via stdin)")
	fmt.Fprintln(w, "  links [slug]      Show link graph stats or specific card links")
	fmt.Fprintln(w, "  backlinks <slug>  Show all cards that link to <slug>")
	fmt.Fprintln(w, "  archive <slug>    Move a card to archive")
	fmt.Fprintln(w, "  organize [since]  Network analysis: orphans, hubs, conflicts, neighbors")
	fmt.Fprintln(w, "  sync [arg]        Sync via git. arg: --init, on, off, or remote URL")
	fmt.Fprintln(w, "  mcp               Start MCP server (stdio transport)")
	fmt.Fprintln(w, "  version           Print version")
}

// memexHome returns $MEMEX_HOME or ~/.memex.
func memexHome() string {
	if h := os.Getenv("MEMEX_HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return filepath.Join(h, ".memex")
}

func newStore() *card.Store {
	home := memexHome()
	return card.NewStore(filepath.Join(home, "cards"), filepath.Join(home, "archive"), false)
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	limit := fs.Int("limit", search.DefaultLimit, "Max results")
	category := fs.String("category", "", "Filter by frontmatter category")
	tag := fs.String("tag", "", "Filter by frontmatter tag")
	author := fs.String("author", "", "Filter by frontmatter author/source")
	since := fs.String("since", "", "Only cards created/modified after this date (YYYY-MM-DD)")
	before := fs.String("before", "", "Only cards created/modified before this date (YYYY-MM-DD)")
	fs.Parse(args)
	query := strings.Join(fs.Args(), " ")
	results, listAll, err := search.Run(newStore(), query, search.Options{
		Limit: *limit,
		Filter: search.Filter{
			Category: *category, Tag: *tag, Author: *author,
			Since: *since, Before: *before,
		},
	})
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}
	if listAll {
		printList(results)
		return nil
	}
	printSearchResults(results)
	return nil
}

func printList(rs []search.Result) {
	maxLen := 0
	for _, r := range rs {
		if len(r.Slug) > maxLen {
			maxLen = len(r.Slug)
		}
	}
	for _, r := range rs {
		fmt.Printf("%s%s\n", padRight(r.Slug, maxLen+2), r.Title)
	}
}

func printSearchResults(rs []search.Result) {
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
	fmt.Println(strings.Join(parts, "\n\n"))
}

func cmdRead(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memex read <slug>")
	}
	raw, err := newStore().ReadCard(args[0])
	if err != nil {
		return err
	}
	os.Stdout.Write(raw)
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func cmdWrite(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memex write <slug>")
	}
	slug := args[0]
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	fm, body := card.ParseFrontmatter(content)
	required := []string{"title", "created", "source"}
	var missing []string
	for _, r := range required {
		if _, ok := fm.Get(r); !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	// Normalize date fields.
	today := todayISO()
	fm.Set("modified", today)
	out := card.StringifyFrontmatter(body, fm)
	if err := newStore().WriteCard(slug, out); err != nil {
		return err
	}
	sync.AutoSync(memexHome())
	return nil
}

func cmdLinks(args []string) error {
	s := newStore()
	if len(args) == 0 {
		m, err := graph.Build(s)
		if err != nil {
			return err
		}
		fmt.Println(graph.FormatStats(m.Stats()))
		return nil
	}
	slug := args[0]
	m, err := graph.Build(s)
	if err != nil {
		return err
	}
	fmt.Println(graph.FormatCardLinks(slug, m.Outbound[slug], m.Inbound[slug]))
	return nil
}

func cmdBacklinks(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memex backlinks <slug>")
	}
	slug := args[0]
	bl, err := graph.Backlinks(newStore(), slug)
	if err != nil {
		return err
	}
	if len(bl) == 0 {
		fmt.Printf("No backlinks found for %s\n", slug)
		return nil
	}
	fmt.Printf("Backlinks for %s:\n", slug)
	for _, b := range bl {
		fmt.Printf("  - %s (cards/)\n", b)
	}
	return nil
}

func cmdArchive(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memex archive <slug>")
	}
	if err := newStore().ArchiveCard(args[0]); err != nil {
		return err
	}
	sync.AutoSync(memexHome())
	return nil
}

func cmdOrganize(args []string) error {
	since := ""
	if len(args) > 0 {
		since = args[0]
	}
	out, err := graph.Organize(newStore(), since)
	if err != nil {
		return err
	}
	fmt.Println(out)
	// Touch .last-organize on success.
	_ = os.WriteFile(filepath.Join(memexHome(), ".last-organize"), []byte(todayISO()+"\n"), 0o644)
	return nil
}

func cmdSync(args []string) error {
	home := memexHome()
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	doInit := fs.Bool("init", false, "Initialize sync")
	doStatus := fs.Bool("status", false, "Show sync status")
	fs.Parse(args)
	rest := fs.Args()
	adapter := sync.New(home)

	// `memex sync on` / `memex sync off`
	if len(rest) == 1 && (rest[0] == "on" || rest[0] == "off") {
		cfg := sync.ReadConfig(home)
		cfg.Auto = rest[0] == "on"
		if err := sync.WriteConfig(home, cfg); err != nil {
			return err
		}
		fmt.Printf("Auto-sync %s.\n", rest[0])
		return nil
	}
	if *doStatus {
		st := adapter.Status()
		fmt.Printf("Configured: %v\nRemote: %s\nAuto: %v\nLast sync: %s\n", st.Remote != "", st.Remote, st.Auto, st.LastSync)
		return nil
	}
	// `memex sync --init [url]` or `memex sync <url>`
	if *doInit || len(rest) == 1 {
		var remote string
		if len(rest) == 1 {
			remote = rest[0]
		}
		return adapter.Init(remote)
	}
	// Plain `memex sync`: pull then push.
	r := adapter.Sync()
	fmt.Println(r.Message)
	if !r.Success {
		os.Exit(1)
	}
	return nil
}

func cmdMCP(_ []string) error {
	return mcp.ServeStdio(newStore(), memexHome(), Version)
}

func todayISO() string {
	return time.Now().UTC().Format("2006-01-02")
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
