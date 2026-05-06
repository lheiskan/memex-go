---
type: feature
stable_id: F-7
slug: bundled-skills-and-install
status: implemented
targets: [REQ-1]
paths:
  - skills/
  - cmd/memex/main.go
---

# Bundled Claude Code skills + `install-skills` subcommand

The upstream npm package shipped 5 SKILL.md files (`memex-recall`,
`memex-retro`, `memex-organize`, `memex-sync`,
`memex-best-practices`) under its own `skills/` directory; users
copied them to `~/.claude/skills/<name>/SKILL.md` once after install
to get the matching `/memex-recall` etc. slash commands.

This feature ports that distribution piece. The Go binary embeds the
same 5 files via `go:embed` and exposes a one-shot `memex
install-skills` subcommand that writes them to `~/.claude/skills/`.

## Surface

- `skills/<name>/SKILL.md` — checked into the repo at the same layout
  upstream used. These are the canonical source; the binary's embed
  is a snapshot taken at build time.
- `package skills` (at repo root) embeds the directory and exports
  `Install(targetDir string) ([]Result, error)`, where each `Result`
  carries the skill name and an `Action` (`Installed` / `Updated` /
  `Unchanged`). The CLI tallies them for the summary line.
- `memex install-skills [--target <dir>]` subcommand — default target
  `~/.claude/skills/`, idempotent overwrite. Reports per-skill action
  (installed / updated / unchanged) and a final tally.

## Behavior

- For each embedded SKILL.md: `mkdir -p <target>/<skill-name>` then
  write `SKILL.md` atomically (temp + rename, like the card store).
- "Unchanged" is detected by reading the existing file and comparing
  bytes; "updated" if existing content differs; "installed" if the
  file didn't exist.
- Doesn't touch any other file in `~/.claude/skills/` — only the 5
  named subdirs.

## Verification

- `skills/skills_test.go::TestInstallToTempDir` runs Install against
  a fresh temp dir and verifies all 5 SKILL.md files land with
  matching content.
- Re-running against the same dir reports all 5 as "unchanged."
