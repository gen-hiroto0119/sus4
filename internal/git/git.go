// Package git wraps the system `git` binary. We deliberately avoid
// go-git: bigger binary, heavier deps, and the user's ~/.gitconfig is
// best honored by reusing their installed git (Design.md §8).
//
// The package is UI-agnostic. Callers receive Go structs only; ANSI
// coloring is intentionally suppressed via --no-color so that
// internal/diffview can apply theme-driven styles.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a handle to a git working tree. Construct via Open.
// A nil *Repo is valid in the rest of the codebase to mean "no repo here";
// see Design.md §3 (`repo *git.Repo  // nil ならば非 git ディレクトリ`).
type Repo struct {
	root string
}

// ErrNotARepo signals the directory has no git working tree.
// Callers should treat this as a soft state, not a fatal error.
var ErrNotARepo = errors.New("git: not a repository")

// Open resolves the working-tree root for dir. Returns ErrNotARepo if
// dir is outside any git repository.
func Open(ctx context.Context, dir string) (*Repo, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if isNotARepoError(err) {
			return nil, ErrNotARepo
		}
		return nil, err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return nil, ErrNotARepo
	}
	return &Repo{root: filepath.Clean(root)}, nil
}

// Root returns the absolute path to the working-tree root.
func (r *Repo) Root() string { return r.root }

// StatusKind classifies a porcelain entry. The granularity matches what
// the sidebar groups by (Design.md §6.1).
type StatusKind int

const (
	StatusModified StatusKind = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
	StatusUntracked
	StatusUnmerged
)

type StatusEntry struct {
	Kind StatusKind
	// Path is repo-relative, slash-separated as git emits it.
	Path string
	// OrigPath is set for renames/copies; empty otherwise.
	OrigPath string
}

// Status returns the working tree's pending changes, parsed from
// `git status --porcelain=v1 -z`.
func (r *Repo) Status(ctx context.Context) ([]StatusEntry, error) {
	out, err := run(ctx, r.root, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	return parseStatusZ(out), nil
}

// DiffWorking returns the unified diff of the working tree against HEAD,
// covering both staged and unstaged changes plus untracked files (added
// via --intent-to-add semantics is out of scope; we use plain `git diff
// HEAD` plus an explicit untracked pass).
func (r *Repo) DiffWorking(ctx context.Context) (string, error) {
	out, err := run(ctx, r.root, "diff", "--no-color", "HEAD")
	if err != nil {
		// Fresh repos with no commits yet: fall back to staged-vs-empty.
		if isNoHEADError(err) {
			out2, err2 := run(ctx, r.root, "diff", "--no-color", "--cached")
			if err2 != nil {
				return "", err2
			}
			return string(out2), nil
		}
		return "", err
	}
	return string(out), nil
}

// DiffFile returns the unified diff for a single repo-relative path.
func (r *Repo) DiffFile(ctx context.Context, path string) (string, error) {
	out, err := run(ctx, r.root, "diff", "--no-color", "HEAD", "--", path)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Show returns the diff that introduced commit-ish.
func (r *Repo) Show(ctx context.Context, commit string) (string, error) {
	out, err := run(ctx, r.root, "show", "--no-color", commit)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// IsTracked reports whether path (repo-relative, slash-separated) is
// known to the index. False covers both untracked files and outright
// non-existent paths — callers that need to distinguish should stat
// the path themselves.
//
// Implementation: `git ls-files --error-unmatch` exits 0 when the path
// is in the index, non-zero otherwise. We swallow stderr because the
// caller treats a missing file as a soft state, not an error to log.
func (r *Repo) IsTracked(ctx context.Context, path string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", "--", path)
	cmd.Dir = r.root
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// HEAD returns the current HEAD ref (short SHA or symbolic name when
// resolvable). Useful for cache invalidation.
func (r *Repo) HEAD(ctx context.Context) (string, error) {
	out, err := run(ctx, r.root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func isNotARepoError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not a git repository")
}

func isNoHEADError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "unknown revision") ||
		strings.Contains(s, "ambiguous argument 'HEAD'") ||
		strings.Contains(s, "bad revision")
}

// parseStatusZ parses git status --porcelain=v1 -z output.
//
// Format per `git-status(1)`: each entry is `XY <path>\0` where X/Y are
// status codes for index/worktree. Renames/copies emit two NULs:
// `XY <to>\0<from>\0`. We collapse the two-axis status into our coarser
// StatusKind by preferring the worktree code, falling back to the index
// code. This is intentionally lossy — the sidebar only needs the bucket.
func parseStatusZ(b []byte) []StatusEntry {
	if len(b) == 0 {
		return nil
	}
	// Trim a possible trailing NUL so Split doesn't yield an empty record.
	if b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	parts := bytes.Split(b, []byte{0})

	var entries []StatusEntry
	for i := 0; i < len(parts); i++ {
		rec := parts[i]
		if len(rec) < 3 {
			continue
		}
		x := rec[0]
		y := rec[1]
		// rec[2] is the separating space.
		path := string(rec[3:])

		kind, isRename := classify(x, y)
		entry := StatusEntry{Kind: kind, Path: path}

		if isRename && i+1 < len(parts) {
			entry.OrigPath = string(parts[i+1])
			i++
		}
		entries = append(entries, entry)
	}
	return entries
}

func classify(x, y byte) (StatusKind, bool) {
	if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
		return StatusUnmerged, false
	}
	if x == '?' && y == '?' {
		return StatusUntracked, false
	}
	if x == 'R' || y == 'R' {
		return StatusRenamed, true
	}
	// Worktree code wins when set; otherwise fall back to the index code.
	c := y
	if c == ' ' || c == 0 {
		c = x
	}
	switch c {
	case 'A':
		return StatusAdded, false
	case 'D':
		return StatusDeleted, false
	case 'M', 'T':
		return StatusModified, false
	}
	return StatusModified, false
}
