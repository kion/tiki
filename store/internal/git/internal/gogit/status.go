package gogit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boolean-maybe/tiki/store/internal/git/internal/types"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ChangedFiles reads the worktree status.
// go-git compares file modes even where the filesystem has none (core.fileMode=false),
// so a worktree change whose content matches the index is not counted.
func (g *Util) ChangedFiles(dirPattern string) (map[string]types.FileChange, error) {
	wt, err := g.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to open worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree status: %w", err)
	}
	pattern := g.toRelativePattern(dirPattern)
	var idx *index.Index
	result := make(map[string]types.FileChange)
	for path, st := range status {
		if !matchesPattern(path, pattern) || st.Staging == git.Untracked || st.Worktree == git.Untracked {
			continue
		}
		switch {
		case st.Staging == git.Unmodified && st.Worktree == git.Unmodified:
			continue
		case st.Staging == git.Deleted || st.Worktree == git.Deleted:
			result[path] = types.FileGone
		case st.Staging == git.Unmodified && g.sameAsIndex(path, &idx):
			continue
		default:
			result[path] = types.FileChanged
		}
	}
	return result, nil
}

func (g *Util) sameAsIndex(path string, idx **index.Index) bool {
	if *idx == nil {
		loaded, err := g.repo.Storer.Index()
		if err != nil {
			return false
		}
		*idx = loaded
	}
	entry, err := (*idx).Entry(path)
	if err != nil {
		return false
	}
	content, err := os.ReadFile(filepath.Join(g.repoPath, filepath.FromSlash(path)))
	if err != nil {
		return false
	}
	return plumbing.ComputeHash(plumbing.BlobObject, content) == entry.Hash
}

// HeadContents reads files from the HEAD commit's tree.
func (g *Util) HeadContents(paths []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	if len(paths) == 0 {
		return result, nil
	}
	head, err := g.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository HEAD: %w", err)
	}
	commit, err := g.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to read HEAD commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to read HEAD tree: %w", err)
	}
	for _, p := range paths {
		f, err := tree.File(p)
		if errors.Is(err, object.ErrFileNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read %s at HEAD: %w", p, err)
		}
		content, err := f.Contents()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s at HEAD: %w", p, err)
		}
		result[p] = []byte(content)
	}
	return result, nil
}
