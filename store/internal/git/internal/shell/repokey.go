package shell

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boolean-maybe/tiki/store/internal/git/internal/types"
)

// RootCommits returns the hashes of the commits without parents reachable from HEAD, sorted.
// A repository without commits has none.
func (u *Util) RootCommits() ([]string, error) {
	verify := exec.Command("git", "rev-parse", "--verify", "-q", "HEAD")
	verify.Dir = u.repoPath
	if err := verify.Run(); err != nil {
		return nil, nil
	}
	// in a shallow clone the boundary commit looks parentless but isn't the root
	shallow := exec.Command("git", "rev-parse", "--is-shallow-repository")
	shallow.Dir = u.repoPath
	if out, err := shallow.Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		return nil, types.ErrShallow
	}
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = u.repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list root commits: %w", err)
	}
	roots := strings.Fields(string(out))
	sort.Strings(roots)
	return roots, nil
}

// CommonDir returns the absolute git directory shared by all worktrees of the repository.
func (u *Util) CommonDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = u.repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find git common dir: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	// older git prints it relative to the working directory
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(u.repoPath, dir)
	}
	return filepath.Clean(dir), nil
}
