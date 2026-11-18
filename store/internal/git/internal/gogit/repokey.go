package gogit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/boolean-maybe/tiki/store/internal/git/internal/types"
)

// RootCommits returns the hashes of the commits without parents reachable from HEAD, sorted.
// A repository without commits has none.
func (g *Util) RootCommits() ([]string, error) {
	head, err := g.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository HEAD: %w", err)
	}
	if shallow, err := g.repo.Storer.Shallow(); err == nil && len(shallow) > 0 {
		return nil, types.ErrShallow
	}
	iter, err := g.repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, fmt.Errorf("list root commits: %w", err)
	}
	var roots []string
	err = iter.ForEach(func(c *object.Commit) error {
		if c.NumParents() == 0 {
			roots = append(roots, c.Hash.String())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list root commits: %w", err)
	}
	sort.Strings(roots)
	return roots, nil
}

// CommonDir returns the absolute git directory shared by all worktrees of the repository.
// It follows git's own layout rules rather than go-git internals:
// a linked worktree's .git file names its gitdir, whose commondir file names the shared one.
func (g *Util) CommonDir() (string, error) {
	dotGit := filepath.Join(g.repoPath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", fmt.Errorf("find git common dir: %w", err)
	}
	if info.IsDir() {
		return filepath.Clean(dotGit), nil
	}
	//nolint:gosec // G304: the repository's own .git file
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", fmt.Errorf("find git common dir: %w", err)
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", fmt.Errorf("find git common dir: unexpected .git file %s", dotGit)
	}
	gitDir = resolveFrom(g.repoPath, strings.TrimSpace(gitDir))
	//nolint:gosec // G304: a file inside the repository's own git dir
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", fmt.Errorf("find git common dir: %w", err)
	}
	return resolveFrom(gitDir, strings.TrimSpace(string(common))), nil
}

func resolveFrom(base, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
