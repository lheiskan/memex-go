---
type: feature
stable_id: F-5
slug: git-sync
status: implemented
targets: [REQ-1]
paths:
  - sync/sync.go
---

# Git-backed cross-device sync via shell-out

Replicates `~/.memex/cards/` (and `archive/`) to a git remote.
Implementation shells out to `git` and `gh` per [[ADR-0004]].

## Operations

- `Init(remote string)` — sets up the git repo, adds the remote
  (auto-creating `memex-cards` on GitHub via `gh` if remote is
  empty), commits cards, attempts to merge any existing remote
  history with `--allow-unrelated-histories`, pushes.
- `Pull()` — `fetch origin` then `merge <detected-branch> --no-edit`.
  Aborts and reports cleanly on merge conflict.
- `Push()` — adds `cards/` and `archive/` only (not the entire
  `~/.memex/`), commits with a timestamped message, pushes.
- `Sync()` — Pull then Push.
- `AutoSync(home)` / `AutoFetch(home)` — silent variants used by the
  MCP hooks. Errors logged to stderr only.

## Config

`~/.memex/.sync.json`:

```json
{
  "remote": "<URL>",
  "adapter": "git",
  "auto": false,
  "lastSync": "<RFC3339>"
}
```

`memex sync on` / `memex sync off` flips the `auto` flag. Auto only
activates if `remote` is also set.

## Branch detection

`detectRemoteBranch` resolves the remote default via
`origin/HEAD` → `origin/main` → `origin/master` → fallback `origin/main`.
Mirrors the JS reference's same probe order.

## Verification

End-to-end exercised against a throwaway GitHub repo on 2026-05-04:
`sync --init <ssh-url>`, `sync` (pull-then-push), `sync --status`,
`sync on`/`sync off` toggle, foreign-commit pull (fast-forward
merge), and the autoSync hook firing on `memex write` with auto on.
Local and remote heads matched after every operation.

Not yet exercised:

- The auto-create-via-`gh` path (`sync --init` with no URL). Would
  create a `memex-cards` private repo on the user's account and is
  the standard onboarding path for new users.
- Conflict resolution (divergent local + remote commits). Edge case
  for single-machine usage.
