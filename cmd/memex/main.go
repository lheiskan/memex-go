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

	memex "memex"
	"memex/card"
	"memex/graph"
	"memex/internal/claudecode"
	"memex/internal/home"
	"memex/mcp"
	"memex/search"
	"memex/status"
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
	case "status":
		err = cmdStatus(args)
	case "harness":
		err = cmdHarness(args)
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
	fmt.Fprintln(w, "memex — Zettelkasten agent memory")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Cards")
	fmt.Fprintln(w, "  search [query]    Search titles and bodies (AND over tokens; 1-3 keywords)")
	fmt.Fprintln(w, "  read <slug>       Print a card")
	fmt.Fprintln(w, "  write <slug>      Write a card; body on stdin, --title/--category/--tags/--source")
	fmt.Fprintln(w, "  archive <slug>    Move a card out of the active set")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Graph")
	fmt.Fprintln(w, "  links [slug]      Link stats, global or for one card")
	fmt.Fprintln(w, "  backlinks <slug>  Cards linking to <slug>")
	fmt.Fprintln(w, "  organize [since]  Orphans, hubs, conflicts, neighbours")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Store")
	fmt.Fprintln(w, "  status            Card count, index sections, days since organize")
	fmt.Fprintln(w, "  sync [arg]        Git sync across machines. arg: --init, on, off, or remote URL")
	fmt.Fprintln(w, "  mcp               Serve the storage tools over MCP on stdio")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Agent setup")
	fmt.Fprintln(w, "  harness           Install memex into an agent harness — `memex harness help`")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  version           Print version")
}

// harnessUsage is a separate screen on purpose: keeping the harness verbs out
// of the top-level listing is what lets the storage surface stay legible.
func harnessUsage(w io.Writer) {
	fmt.Fprintln(w, "memex harness — install memex into an LLM agent harness")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  install           Write the skills, the session hook, the MCP registration")
	fmt.Fprintln(w, "  check             Report what is installed, where it points, whether it is current")
	fmt.Fprintln(w, "  uninstall         Remove only what install wrote")
	fmt.Fprintln(w, "  session-start     Print the session-start directive (what the hook invokes)")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  --target NAME     Agent harness to install into. Supported: %s\n", supportedList())
	fmt.Fprintln(w, "  --dir PATH        Config directory of the target harness")
	fmt.Fprintln(w, "                    (default: the target's own, e.g. ~/.claude; or $CLAUDE_DIR)")
	fmt.Fprintln(w, "  --dry-run         Write nothing; report what would happen")
	fmt.Fprintln(w, "  --skip-mcp        Leave the MCP registration alone")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "`check` exits non-zero when anything needs installing, so it is scriptable.")
}

// memexHome returns $MEMEX_HOME or ~/.memex.
func memexHome() string {
	h, err := home.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return h
}

func newStore() *card.Store {
	return home.Store(memexHome())
}

// parseFlags parses args allowing flags and positionals in any order, and
// returns the positionals.
//
// Go's flag package stops at the first non-flag argument, which makes
// `memex search docker --limit 3` parse as the single query
// "docker --limit 3" — a query that matches nothing, printed as an empty
// result that reads exactly like "the memory has nothing on this". An agent
// driving this CLI from a skill types the flags where prose puts them, so the
// footgun has to go rather than be documented.
//
// `--` is a hard terminator: everything after it is a positional verbatim,
// so a query token that begins with a dash is still reachable.
func parseFlags(fs *flag.FlagSet, args []string) []string {
	var tail []string
	for i, a := range args {
		if a == "--" {
			args, tail = args[:i], args[i+1:]
			break
		}
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil // ExitOnError has already exited; be defensive anyway.
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	return append(positional, tail...)
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	limit := fs.Int("limit", search.DefaultLimit, "Max results")
	category := fs.String("category", "", "Filter by frontmatter category")
	tag := fs.String("tag", "", "Filter by frontmatter tag")
	author := fs.String("author", "", "Filter by frontmatter author/source")
	since := fs.String("since", "", "Only cards created/modified after this date (YYYY-MM-DD)")
	before := fs.String("before", "", "Only cards created/modified before this date (YYYY-MM-DD)")
	query := strings.Join(parseFlags(fs, args), " ")
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

// defaultSource is stamped on a card that does not say where it came from.
// Claude Code is the only supported harness, so this is a fact rather than a
// guess.
const defaultSource = "claude-code"

// cmdWrite writes a card. The frontmatter policy lives in card.Compose so that
// this path and the MCP memex_write tool cannot drift apart.
func cmdWrite(args []string) error {
	fs := flag.NewFlagSet("write", flag.ExitOnError)
	title := fs.String("title", "", "Card title")
	category := fs.String("category", "", "Card category")
	tags := fs.String("tags", "", "Comma-separated tags")
	source := fs.String("source", "", "What produced this card (default "+defaultSource+")")
	rest := parseFlags(fs, args)
	if len(rest) < 1 {
		return fmt.Errorf("usage: memex write <slug> [--title T] [--category C] [--tags a,b] [--source S]")
	}
	slug := rest[0]
	incoming, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	store := newStore()
	existing, _ := store.ReadCard(slug) // nil when the card is new

	out, err := card.Compose(incoming, existing, card.AuthorOptions{
		Title:         *title,
		Category:      *category,
		Tags:          *tags,
		Source:        *source,
		DefaultSource: defaultSource,
		Today:         todayISO(),
	})
	if err != nil {
		return fmt.Errorf("%w: pass --title, or include frontmatter on stdin", err)
	}
	if err := store.WriteCard(slug, out); err != nil {
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
	return nil
}

func cmdSync(args []string) error {
	home := memexHome()
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	doInit := fs.Bool("init", false, "Initialize sync")
	doStatus := fs.Bool("status", false, "Show sync status")
	rest := parseFlags(fs, args)
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

// cmdStatus prints facts about the store. The prose that wraps them belongs to
// whichever harness is being told, so it is not here.
func cmdStatus(_ []string) error {
	fmt.Print(status.Format(status.Collect(newStore(), memexHome())))
	return nil
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

// supportedHarnesses is what --target accepts.
//
// A list of one is deliberate, and it is not the pluggable-installer seam this
// project has declined three times: there is no interface and no registry of
// implementations, just a validated argument. What it buys is that the
// constraint lives in `harness help` and in the error message rather than in a
// prose sentence that goes stale the moment the list changes.
var supportedHarnesses = []string{"claude-code"}

const defaultHarness = "claude-code"

// supportedList renders the accepted values, reading correctly whether there
// is one of them or several.
func supportedList() string {
	if len(supportedHarnesses) == 1 {
		return supportedHarnesses[0] + " (the default)"
	}
	return strings.Join(supportedHarnesses, ", ") + ", default " + defaultHarness
}

// validateTarget rejects an unknown harness by name, listing what is accepted.
// Without this, `--target cursor` would silently install Claude Code's layout.
func validateTarget(name string) error {
	for _, h := range supportedHarnesses {
		if name == h {
			return nil
		}
	}
	return fmt.Errorf("unknown harness %q (supported: %s)",
		name, strings.Join(supportedHarnesses, ", "))
}

// cmdHarness dispatches the `harness` namespace.
//
// Everything that knows Claude Code exists lives behind this one verb. The
// dividing line is vocabulary: `session-start` is a Claude Code hook event
// name so it belongs here, while `memex mcp` stays top-level because MCP is a
// protocol that other clients speak too.
func cmdHarness(args []string) error {
	if len(args) == 0 {
		harnessUsage(os.Stderr)
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("harness "+sub, flag.ExitOnError)
	target := fs.String("target", defaultHarness, "Agent harness to target")
	dir := fs.String("dir", os.Getenv("CLAUDE_DIR"), "Harness config directory")
	dryRun := fs.Bool("dry-run", false, "Write nothing")
	skipMCP := fs.Bool("skip-mcp", false, "Leave the MCP registration alone")
	noSync := fs.Bool("no-sync", false, "session-start: skip the card sync")
	parseFlags(fs, rest)
	if err := validateTarget(*target); err != nil {
		return err
	}

	opts := claudecode.Options{ClaudeDir: *dir, DryRun: *dryRun, SkipMCP: *skipMCP}

	switch sub {
	case "install":
		return harnessInstall(opts)
	case "check":
		return harnessCheck(opts)
	case "uninstall":
		return harnessUninstall(opts)
	case "session-start":
		return harnessSessionStart(*noSync)
	case "help", "--help", "-h":
		harnessUsage(os.Stdout)
		return nil
	}
	fmt.Fprintf(os.Stderr, "unknown harness command: %s\n\n", sub)
	harnessUsage(os.Stderr)
	os.Exit(2)
	return nil
}

func say(a claudecode.Action, detail string) {
	fmt.Printf("  %-12s %s\n", a, detail)
}

func harnessInstall(opts claudecode.Options) error {
	settings, err := claudecode.SettingsPath(opts.ClaudeDir)
	if err != nil {
		return err
	}

	fmt.Printf("Skills -> %s\n", filepath.Join(filepath.Dir(settings), "skills"))
	steps, err := claudecode.InstallSkills(memex.Skills, opts)
	if err != nil {
		return err
	}
	for _, s := range steps {
		say(s.Action, s.Detail)
	}

	fmt.Printf("Hook -> %s\n", settings)
	step, foreign, err := claudecode.InstallHook(opts)
	if err != nil {
		return err
	}
	say(step.Action, step.Detail)
	warnForeign(foreign)

	mcpCmd, err := claudecode.SelfCommand(claudecode.MCPSubcommand)
	if err != nil {
		return err
	}
	claudecode.RegisterMCP(os.Stdout, mcpCmd, opts)

	fmt.Println()
	fmt.Println("Done. Start a new Claude Code session to pick up the hook and skills.")
	return nil
}

// harnessCheck exits non-zero when anything needs installing, so a script or
// a CI step can gate on it.
func harnessCheck(opts claudecode.Options) error {
	report, err := claudecode.Check(memex.Skills, opts)
	if err != nil {
		return err
	}
	fmt.Print(claudecode.FormatReport(report))
	if !report.OK() {
		os.Exit(1)
	}
	return nil
}

func harnessUninstall(opts claudecode.Options) error {
	settings, err := claudecode.SettingsPath(opts.ClaudeDir)
	if err != nil {
		return err
	}
	fmt.Printf("Removing from %s\n", filepath.Dir(settings))
	steps, foreign, err := claudecode.Uninstall(memex.Skills, opts)
	if err != nil {
		return err
	}
	for _, s := range steps {
		say(s.Action, s.Detail)
	}
	warnForeign(foreign)
	claudecode.UnregisterMCP(os.Stdout, opts)
	return nil
}

// harnessSessionStart is what the installed hook runs. It always exits 0: a
// hook that fails must not break a session, and its stdout goes into model
// context, so sync noise must never reach it.
func harnessSessionStart(noSync bool) error {
	dir, err := home.Dir()
	if err != nil {
		return nil
	}
	if !noSync {
		if cfg := sync.ReadConfig(dir); cfg.Remote != "" {
			sync.New(dir).Sync()
		}
	}
	fmt.Print(claudecode.SessionStartText(status.Collect(home.Store(dir), dir)))
	return nil
}

func warnForeign(foreign []string) {
	for _, c := range foreign {
		fmt.Fprintf(os.Stderr, "warning: another memex SessionStart hook is present: %s\n", c)
		fmt.Fprintln(os.Stderr, "         It was left untouched — remove it to avoid duplicate instructions.")
	}
}
