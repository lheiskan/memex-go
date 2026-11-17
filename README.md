# memex

Zettelkasten inspired agent memory as a single static binary, implemented in
Go. Derived from [`@touchskyer/memex`](https://github.com/iamtouchskyer/memex).

## Build and install

Requires Go 1.25.5+. No build dependencies; `git` at runtime for `memex sync`,
and `gh` if you want `--init` to create the remote. macOS, Linux, Windows.

```sh
# Ensure $GOPATH/bin is on $PATH, or set $GOBIN to a location that is.
go install ./cmd/memex

# Install the Claude Code integration: skills, session-start hook,
# MCP server registration. Needs no repo checkout of its own.
memex harness install            # --dry-run shows what it would do
memex harness check              # is my install current?

# Optionally enable change tracking and sync between devices via git
memex sync --init https://github.com/<you>/<repo>.git
memex sync on             # commit + push on every write/archive
memex sync --status
```

## Usage

Memory is meant to be used **from your agent**, not from the shell. After
`memex harness install`, the agent has five skills it can invoke as slash
commands — and it reaches for two of them on its own.

| | When to use it |
| --- | --- |
| `/memex-recall` | Before work that overlaps something you have done before — debugging a familiar area, continuing a project, revisiting an old decision. **The agent does this itself** when memory looks relevant, so you rarely need to ask. |
| `/memex-retro` | After work involving code changes, debugging, or a non-trivial decision. **The agent does this itself** at task end: it drafts the cards, names them, and leaves you to decide what is kept. |
| `/memex-organize` | Periodically, to keep the card network healthy: surface conflicting cards for you to resolve, archive stale ones, link related ones, and rebuild the keyword index that recall uses as its entry point. Ask for it. |
| `/memex-sync` | To set up or run git sync across devices. Ask for it. |
| `/memex-best-practices` | When you want to know how a good card is written — atomicity, naming, linking. A reference the agent reads, not an action it performs. |

So in normal use you type nothing: the agent recalls what is relevant before
working and proposes cards afterwards. The three on-request skills are for
maintenance and setup.

## CLI reference

The CLI is for installation and tooling. Day-to-day memory use goes through
the agent, so you should not need these directly.

| | |
| --- | --- |
| `memex search [query]` | Search titles and bodies. AND over tokens, so use 1-3 keywords. |
| `memex read <slug>` | Print a card. |
| `memex write <slug>` | Write a card. Body on stdin; `--title`, `--category`, `--tags`, `--source` author the frontmatter. |
| `memex links [slug]` / `backlinks <slug>` | Link stats, or one card's links. |
| `memex organize [since]` | Orphans, hubs, contradictions, neighbours. |
| `memex archive <slug>` | Move a card out of the active set. |
| `memex sync [--init\|on\|off\|--status]` | Git sync across machines. |
| `memex mcp` | Serve the eight storage tools over MCP on stdio. |
| `memex status` | Store facts: card count, index sections, days since organize. |

Agent setup lives under one namespace, `memex harness`:

| | |
| --- | --- |
| `memex harness install` | Skills, the SessionStart hook, the MCP registration. Idempotent. |
| `memex harness check` | What is installed, where it points, whether it is current. Exits non-zero if anything needs installing. |
| `memex harness uninstall` | Removes only what install wrote. |
| `memex harness session-start` | The session-start directive. What the installed hook runs. |

Flags: `--target NAME` (supported harnesses; `memex harness help` lists them), `--dir PATH`, `--dry-run`, `--skip-mcp`.

## Storage

Cards live under `~/.memex` by default; set `MEMEX_HOME` to point elsewhere.

```sh
MEMEX_HOME=/tmp/memex-test memex search
```

Cards are plain Markdown with YAML frontmatter, so nothing is hidden behind
the binary: `grep`, an editor, or a notes app syncing the same directory all
work on the store directly.
