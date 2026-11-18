package git

import "time"

// GitOps defines the interface for git operations
type GitOps interface {
	Add(paths ...string) error
	Remove(paths ...string) error
	CurrentUser() (name string, email string, err error)
	Author(filePath string) (*AuthorInfo, error)
	AllAuthors(dirPattern string) (map[string]*AuthorInfo, error)
	LastCommitTime(filePath string) (time.Time, error)
	AllLastCommitTimes(dirPattern string) (map[string]time.Time, error)
	CurrentBranch() (string, error)
	FileVersionsSince(filePath string, since time.Time, includePrior bool) ([]FileVersion, error)
	AllFileVersionsSince(dirPattern string, since time.Time, includePrior bool) (map[string][]FileVersion, error)
	AllUsers() ([]string, error)
	// RepoRelPath returns the key AllAuthors and AllLastCommitTimes use for path.
	RepoRelPath(path string) (string, error)
	// ChangedFiles reports tracked files under dirPattern that differ from HEAD or are gone;
	// untracked files are left out.
	ChangedFiles(dirPattern string) (map[string]FileChange, error)
	// HeadContents returns the contents at HEAD of repo-relative paths; paths missing there are left out.
	HeadContents(paths []string) (map[string][]byte, error)
	// RootCommits returns the sorted hashes of parentless commits reachable from HEAD; none without commits.
	RootCommits() ([]string, error)
	// CommonDir returns the absolute git directory shared by every worktree of the repository.
	CommonDir() (string, error)
}

// NewGitOps creates a new GitOps instance. Backend selection (shell vs go-git)
// is deferred until the first method call, based on whether shell git is available.
func NewGitOps(repoPath string) (GitOps, error) {
	return newSelector(repoPath), nil
}
