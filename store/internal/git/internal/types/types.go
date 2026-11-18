package types

import (
	"errors"
	"time"
)

// AuthorInfo contains information about who created a file
type AuthorInfo struct {
	Name       string
	Email      string
	Date       time.Time
	CommitHash string
	Message    string
}

// FileVersion represents the content of a file at a specific commit
type FileVersion struct {
	Hash    string
	Author  string
	Email   string
	When    time.Time
	Content string
}

// FileChange is how a tracked file differs from HEAD.
type FileChange uint8

const (
	// FileChanged is tracked and differs from HEAD in the index or the working tree.
	FileChanged FileChange = iota + 1
	// FileGone is tracked at HEAD but missing from the working tree or the index.
	FileGone
)

// ErrShallow means a shallow clone can't name its repository's first commit.
var ErrShallow = errors.New("this is a shallow clone, so its first commit is unknown; run `git fetch --unshallow` first")
