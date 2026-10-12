package document

import (
	"os"
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

// For callers that already know the directory isn't the root.
func isSkippableDirName(name string) bool {
	if name == docDirName {
		return false
	}
	return strings.HasPrefix(name, ".")
}

// FindIDPath locates a tiki without loading the store,
// so commands that only need to know a tiki exists skip git and workflow loading.
func FindIDPath(root, id string) (string, bool, error) {
	paths, err := WalkDocuments(root)
	if err != nil {
		return "", false, err
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := ParseFrontmatter(string(content))
		if err != nil {
			continue
		}
		if raw, ok := FrontmatterID(parsed.Map); ok && NormalizeID(raw) == id {
			return path, true, nil
		}
	}
	return "", false, nil
}
