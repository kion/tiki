package shell

import (
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Util provides Git operations by shelling out to git commands
type Util struct {
	repoPath    string
	cachedUsers []string

	currentUserMu     sync.Mutex
	currentUserCached bool
	currentUserName   string
	currentUserEmail  string
	currentUserErr    error

	prefixOnce sync.Once
	prefix     string
	prefixErr  error
}

// RepoRelPath returns path relative to the repo root, slash-separated.
func (u *Util) RepoRelPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", path, err)
	}
	base, err := filepath.Abs(u.repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", u.repoPath, err)
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", fmt.Errorf("failed to convert path %s to relative: %w", path, err)
	}
	// Prefix from git rather than Rel against the toplevel:
	// git reports the toplevel with symlinks resolved, but repoPath may run through one.
	u.prefixOnce.Do(func() {
		cmd := exec.Command("git", "rev-parse", "--show-prefix")
		cmd.Dir = u.repoPath
		out, err := cmd.Output()
		u.prefix, u.prefixErr = strings.TrimSpace(string(out)), err
	})
	if u.prefixErr != nil {
		return "", fmt.Errorf("failed to get repo prefix: %w", u.prefixErr)
	}
	return pathpkg.Join(u.prefix, filepath.ToSlash(rel)), nil
}

// NewUtil creates a new Util instance; repoPath defaults to cwd
func NewUtil(repoPath string) (*Util, error) {
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("not a git repository at %s: %w", repoPath, err)
	}

	return &Util{repoPath: repoPath}, nil
}

// CurrentBranch returns the name of the currently active branch
func (u *Util) CurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = u.repoPath
	output, err := cmd.Output()
	if err != nil {
		cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
		cmd.Dir = u.repoPath
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get current branch: %w", err)
		}
		return strings.TrimSpace(string(output)), nil
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
		cmd.Dir = u.repoPath
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get HEAD hash: %w", err)
		}
		return strings.TrimSpace(string(output)), nil
	}

	return branch, nil
}

func (u *Util) toRelative(path string) (string, error) {
	if filepath.IsAbs(path) {
		relPath, err := filepath.Rel(u.repoPath, path)
		if err != nil {
			return "", fmt.Errorf("failed to convert path %s to relative: %w", path, err)
		}
		return relPath, nil
	}
	return path, nil
}

func parseGitTime(dateStr string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 -07:00",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse git time %q", dateStr)
}
