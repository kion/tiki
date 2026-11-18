package git

import "github.com/boolean-maybe/tiki/store/internal/git/internal/types"

// AuthorInfo contains information about who created a file
type AuthorInfo = types.AuthorInfo

// FileVersion represents the content of a file at a specific commit
type FileVersion = types.FileVersion

// FileChange is how a tracked file differs from HEAD.
type FileChange = types.FileChange

// The alias above doesn't carry its constants along.
const (
	FileChanged = types.FileChanged
	FileGone    = types.FileGone
)
