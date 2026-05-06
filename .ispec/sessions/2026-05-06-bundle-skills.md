---
type: session
slug: 2026-05-06-bundle-skills
trigger: bundle upstream's claude-code skills into the go binary distribution
classification: new-feature-requirement-exists
status: resolved
affected_cards:
  - REQ-1
  - F-6
  - F-7
deferred_cards: []
commits:
  - f05cd66
outcome: |
  Bundled the 5 upstream SKILL.md files into the Go binary via
  go:embed and wired up `memex install-skills`. New feature card F-7
  (`bundled-skills-and-install`); F-6 amended with the new subcommand
  row. Smoke-test against a temp dir confirmed install + idempotent
  re-install + byte-identical to the user's existing
  ~/.claude/skills/memex-* set. Lint clean (18 cards, 0 errors).

  One reconciliation update: F-7's drafted Install signature was
  `(installed, updated int, err)`; the implementation needed
  per-skill action info for the CLI summary, so it returns `[]Result`
  instead. Canon updated to match the better API.

  Future maintenance note (captured in Discoveries): the
  `//go:embed` directive lists skill names explicitly; adding a 6th
  skill requires editing skills/skills.go. TestNames pins the count
  at 5 so a forgotten edit fails CI.
---

## Trigger

The user's `~/.claude/skills/memex-*/` directories were copied from
the npm package's `skills/` dir at install time. The user has since
uninstalled the npm package — those skill files are now orphaned (no
source-of-truth they came from, no upgrade path). Bundling the skills
inside the Go binary plus a one-shot `install-skills` subcommand makes
the port a true drop-in: a fresh user runs `memex install-skills`
once and gets the same `/memex-recall`, `/memex-retro`, etc.
slash-commands the upstream tool provided.

## Pre-decisions

- Skills live at `skills/<name>/SKILL.md` at the repo root (mirrors
  upstream's layout; visible in the working tree).
- An `internal/skills` package would be more idiomatic, but the
  user-facing repo browse benefits from skills/ sitting at the top
  level. Use `package skills` at the root and `//go:embed all:.`
  inside that package's `skills.go`.
- Subcommand: `memex install-skills`, default target `~/.claude/skills/`,
  optional `--target <dir>` for testing. Idempotent — always
  overwrites (file content is canonical from the binary).
- No skill-content edits — `grep` found no upstream-specific text
  (npm/memra/openai/llama). Files copy verbatim from the live
  `~/.claude/skills/memex-*/SKILL.md` set.

## Discoveries during the session

- `//go:embed` requires the embedded paths to be under the embedding
  Go file's directory tree. To keep `skills/` visible at the repo
  root (alongside `.ispec/`, `card/`, etc.) instead of hiding it
  under `cmd/memex/`, the embedding lives in a `package skills` at
  the repo root.
- The directive doesn't accept a single glob like `all:*` — it
  resolves at compile time and needs the directory names listed
  explicitly. If a skill is added later, the `//go:embed` line in
  `skills/skills.go` needs updating; otherwise the new skill is
  silently absent. Tests would catch a missing one only if they
  asserted a specific count or set of names — which `TestNames`
  now does (asserts `len == 5`).
- The drafted `Install(targetDir) (installed, updated int, err)`
  signature was wrong — the CLI needed per-skill action info to
  print the line-by-line summary, so the implementation returns
  `[]Result` instead. Canon updated to match (reconciliation step).

## Outcome

(filled at session close)
