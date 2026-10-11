package document

import (
	"sort"
	"strings"
)

// WalkDocuments walks root recursively and returns every file path that
// should be treated as a managed markdown document. It is the single
// source of truth for "which files on disk count as documents", shared by
// the runtime store loader and any future maintenance tooling.
//
// Exclusion rules (see per-helper godoc for specifics):
//
//   - non-`.md` files are ignored;
//   - hidden subdirectories (`.git`, `.idea`, `.obsidian`, ...) are pruned,
//     except `.doc` (the legacy document store), which is always traversed;
//   - paths matched by `.gitignore` or `.tikiignore` at root are excluded.
//
// Returned paths are sorted so callers that iterate them get deterministic
// ordering for diagnostics and duplicate-id reporting.
func WalkDocuments(root string) ([]string, error) {
	scope, err := NewScope(root)
	if err != nil {
		return nil, err
	}
	var paths []string
	walkErr := scope.Walk(scope.Root(), func(path string, isDir bool) error {
		if !isDir {
			paths = append(paths, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(paths)
	return paths, nil
}

// docDirName is the one hidden directory the walker descends into: the legacy
// document store. Projects predating the cwd-rooted scan keep their tikis under
// `.doc/`, so it is exempt from the hidden-directory prune at any depth.
const docDirName = ".doc"

// IsSkippableSubdir reports whether a directory encountered during a walk
// should be pruned. The root itself is never skipped. Hidden names (names
// starting with ".") are skipped — they are editor/VCS metadata (`.git`,
// `.obsidian`, `.idea`, ...) — with the sole exception of `.doc`, the legacy
// document store, which is traversed wherever it appears.
func IsSkippableSubdir(path, root, name string) bool {
	if path == root {
		return false
	}
	return isSkippableDirName(name)
}

// isSkippableDirName is the name-only half of IsSkippableSubdir, for callers
// that already know the directory is not the root.
func isSkippableDirName(name string) bool {
	if name == docDirName {
		return false
	}
	return strings.HasPrefix(name, ".")
}
