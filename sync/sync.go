// Package sync wraps the `git` and `gh` CLIs to keep ~/.memex synchronised
// across devices.
package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const configFile = ".sync.json"

type Config struct {
	Remote   string `json:"remote,omitempty"`
	Adapter  string `json:"adapter,omitempty"`
	Auto     bool   `json:"auto"`
	LastSync string `json:"lastSync,omitempty"`
}

func ReadConfig(home string) Config {
	raw, err := os.ReadFile(filepath.Join(home, configFile))
	if err != nil {
		return Config{Adapter: "git"}
	}
	var c Config
	if json.Unmarshal(raw, &c) != nil {
		return Config{Adapter: "git"}
	}
	return c
}

func WriteConfig(home string, c Config) error {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(home, configFile), b, 0o644)
}

// Adapter wraps git operations against home (~/.memex).
type Adapter struct {
	Home string
}

func New(home string) *Adapter { return &Adapter{Home: home} }

func (a *Adapter) git(args ...string) (string, error) {
	full := append([]string{"-C", a.Home}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitAvailable() bool {
	return exec.Command("git", "--version").Run() == nil
}

func ghAvailable() bool {
	return exec.Command("gh", "--version").Run() == nil
}

// detectRemoteBranch returns "origin/<branch>" — the default branch on remote.
func (a *Adapter) detectRemoteBranch() string {
	if out, err := a.git("rev-parse", "--abbrev-ref", "origin/HEAD"); err == nil {
		out = strings.TrimSpace(out)
		if out != "" && out != "origin/HEAD" {
			return out
		}
	}
	for _, candidate := range []string{"origin/main", "origin/master"} {
		if _, err := a.git("rev-parse", "--verify", candidate); err == nil {
			return candidate
		}
	}
	return "origin/main"
}

type Result struct {
	Success bool
	Message string
}

// Init creates or links a git remote at home and pushes the initial commit.
// If remote is empty, requires `gh` CLI to auto-create a private memex-cards repo.
func (a *Adapter) Init(remote string) error {
	if !gitAvailable() {
		return fmt.Errorf("git is required for sync. Install git first")
	}
	url := remote
	if url == "" {
		if !ghAvailable() {
			return fmt.Errorf("provide a repo URL or install gh CLI (https://cli.github.com)")
		}
		if err := exec.Command("gh", "auth", "status").Run(); err != nil {
			return fmt.Errorf("gh CLI is not authenticated. Run `gh auth login` first")
		}
		// Reuse existing memex-cards repo if present.
		if out, err := exec.Command("gh", "repo", "view", "memex-cards", "--json", "url", "-q", ".url").Output(); err == nil {
			url = strings.TrimSpace(string(out))
		} else {
			out, err := exec.Command("gh", "repo", "create", "memex-cards", "--private").Output()
			if err != nil {
				return fmt.Errorf("gh repo create failed: %w", err)
			}
			url = strings.TrimSpace(string(out))
		}
		if url == "" {
			return fmt.Errorf("failed to get repo URL from gh CLI")
		}
	}

	if _, err := a.git("rev-parse", "--git-dir"); err != nil {
		if _, err := exec.Command("git", "init", a.Home).CombinedOutput(); err != nil {
			return fmt.Errorf("git init failed: %w", err)
		}
	}
	_, _ = a.git("config", "core.autocrlf", "false")

	gitignorePath := filepath.Join(a.Home, ".gitignore")
	wanted := []string{".sync.json", ".last-organize"}
	if existing, err := os.ReadFile(gitignorePath); err == nil {
		var missing []string
		for _, e := range wanted {
			if !strings.Contains(string(existing), e) {
				missing = append(missing, e)
			}
		}
		if len(missing) > 0 {
			combined := strings.TrimRight(string(existing), "\n") + "\n" + strings.Join(missing, "\n") + "\n"
			_ = os.WriteFile(gitignorePath, []byte(combined), 0o644)
		}
	} else {
		_ = os.WriteFile(gitignorePath, []byte(strings.Join(wanted, "\n")+"\n"), 0o644)
	}

	if _, err := a.git("remote", "add", "origin", url); err != nil {
		// Probably "remote origin already exists" — try set-url instead.
		_, _ = a.git("remote", "set-url", "origin", url)
	}

	_, _ = a.git("add", "cards")
	_, _ = a.git("add", "archive")
	_, _ = a.git("commit", "-m", "memex: initial sync")

	if _, err := a.git("fetch", "origin"); err == nil {
		branch := a.detectRemoteBranch()
		_, _ = a.git("merge", branch, "--allow-unrelated-histories", "--no-edit")
	}
	if _, err := a.git("push", "-u", "origin", "HEAD"); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return WriteConfig(a.Home, Config{Remote: url, Adapter: "git"})
}

func (a *Adapter) Pull() Result {
	if ReadConfig(a.Home).Remote == "" {
		return Result{Success: false, Message: "Not configured."}
	}
	if _, err := a.git("fetch", "origin"); err != nil {
		return Result{Success: true, Message: "Offline, using local data."}
	}
	branch := a.detectRemoteBranch()
	if _, err := a.git("merge", branch, "--no-edit"); err != nil {
		_, _ = a.git("merge", "--abort")
		return Result{
			Success: false,
			Message: fmt.Sprintf("Merge conflict. Run `cd %s && git status` to see conflicting files, resolve them, then `git add . && git commit`.", a.Home),
		}
	}
	return Result{Success: true, Message: "Pulled latest."}
}

func (a *Adapter) Push() Result {
	cfg := ReadConfig(a.Home)
	if cfg.Remote == "" {
		return Result{Success: false, Message: "Not configured."}
	}
	_, _ = a.git("add", "cards")
	_, _ = a.git("add", "archive")
	_, _ = a.git("commit", "-m", fmt.Sprintf("memex sync %s", time.Now().UTC().Format(time.RFC3339)))
	if out, err := a.git("push", "origin", "HEAD"); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("Push failed: %s", out)}
	}
	cfg.LastSync = time.Now().UTC().Format(time.RFC3339)
	_ = WriteConfig(a.Home, cfg)
	return Result{Success: true, Message: "Pushed."}
}

func (a *Adapter) Sync() Result {
	if r := a.Pull(); !r.Success {
		return r
	}
	return a.Push()
}

// AutoSync runs a full sync if the config has both a remote and auto=true.
// Errors are swallowed (logged only) — sync is infrastructure, not business logic.
func AutoSync(home string) {
	cfg := ReadConfig(home)
	if !cfg.Auto || cfg.Remote == "" {
		return
	}
	r := New(home).Sync()
	if !r.Success {
		fmt.Fprintf(os.Stderr, "sync warning: %s\n", r.Message)
	}
}

// AutoFetch runs only the pull half if a remote is configured. Silent on failure.
func AutoFetch(home string) {
	if ReadConfig(home).Remote == "" {
		return
	}
	_ = New(home).Pull()
}

// Status returns a snapshot of sync state, self-healing .sync.json's `remote`
// field from `git remote get-url origin` if it's missing.
func (a *Adapter) Status() Config {
	cfg := ReadConfig(a.Home)
	if cfg.Remote == "" {
		if out, err := a.git("remote", "get-url", "origin"); err == nil && out != "" {
			cfg.Remote = out
			_ = WriteConfig(a.Home, cfg)
		}
	}
	return cfg
}
