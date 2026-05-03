---
type: adr
stable_id: ADR-0004
slug: shell-out-to-git-for-sync
status: accepted
relates_to: [REQ-1, REQ-2]
---

# Shell out to git/gh CLIs for sync, don't link a Go git library

## Context

Memex sync replicates `~/.memex/cards/` to a git remote (typically
auto-created as a private `memex-cards` GitHub repo via `gh`). The
operations needed are limited: `init`, `add`, `commit`, `fetch`,
`merge`, `push`, plus `gh repo view/create` for the bootstrap path.

Two implementation options:

1. Link a Go git library (`go-git/go-git`) — pure-Go, no external
   dependency, but adds a substantial transitive surface and
   re-implements git's behavior (sometimes diverging on edge cases).
2. Shell out to the `git` CLI via `os/exec`, exactly mirroring the
   JS reference's approach (which uses Node's `execFile`).

## Decision

Shell out to `git` and `gh`. Wrap them in a thin `Adapter` type
(`sync/sync.go`) with one method per operation. `gh` is only required
for the auto-create-repo path of `memex sync --init`; if the user
provides a remote URL explicitly, only `git` is needed.

## Consequences

- Dependency-free sync layer (no `go-git` transitives, per [[REQ-2]]).
- Behavior matches the user's expectation precisely — if `git` works
  in their shell, sync works the same way. Standard ssh-agent,
  `~/.gitconfig`, credential helpers all just work.
- Errors are exit codes + stderr captured via `CombinedOutput`. Less
  ergonomic than typed library errors, but easier to debug ("here's
  exactly what `git` said when I ran it").
- Requires `git` (always present on the dev's machine) and `gh` (only
  for `--init` without an explicit URL). Documented in the README /
  status output.
- Untested at write time — the Init/Pull/Push paths shell out to a
  real remote and weren't exercised in the initial implementation
  because doing so would push to the user's actual GitHub. Need at
  least one full sync round on a throwaway remote before committing
  the binary as the canonical sync driver.
