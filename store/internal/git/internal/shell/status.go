package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/boolean-maybe/tiki/store/internal/git/internal/types"
)

// --no-optional-locks keeps a read from rewriting the index under a concurrent commit,
// and --no-renames gives one path per entry whatever the user's status.renames.
func (u *Util) ChangedFiles(dirPattern string) (map[string]types.FileChange, error) {
	//nolint:gosec // G204: git command with controlled directory pattern
	cmd := exec.Command("git", "--no-optional-locks", "-c", "core.quotePath=false", "status",
		"--porcelain=v1", "-z", "--untracked-files=no", "--no-renames", "--ignore-submodules=all", "--", dirPattern)
	cmd.Dir = u.repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status for %s: %w", dirPattern, err)
	}
	result := make(map[string]types.FileChange)
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) < 4 || entry[0] == '?' || entry[0] == '!' {
			continue
		}
		path := string(entry[3:])
		if entry[0] == 'D' || entry[1] == 'D' {
			result[path] = types.FileGone
		} else {
			result[path] = types.FileChanged
		}
	}
	return result, nil
}

// HeadContents reads all paths with one `git cat-file --batch`.
func (u *Util) HeadContents(paths []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	var input bytes.Buffer
	var asked []string
	for _, p := range paths {
		// a newline would split the request line and misalign every answer after it
		if strings.Contains(p, "\n") {
			continue
		}
		input.WriteString("HEAD:" + p + "\n")
		asked = append(asked, p)
	}
	if len(asked) == 0 {
		return result, nil
	}
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = u.repoPath
	cmd.Stdin = &input
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read files at HEAD: %w", err)
	}
	r := bufio.NewReader(bytes.NewReader(out))
	for _, p := range asked {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading cat-file output: %w", err)
		}
		if strings.HasSuffix(header, " missing\n") || strings.HasSuffix(header, " ambiguous\n") {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected cat-file header %q", header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("unexpected cat-file header %q", header)
		}
		body := make([]byte, size+1)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("reading cat-file output: %w", err)
		}
		if fields[1] == "blob" {
			result[p] = body[:size]
		}
	}
	return result, nil
}
