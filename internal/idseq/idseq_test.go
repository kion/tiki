package idseq

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func hash8(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:8]
}

func TestFileName(t *testing.T) {
	if got, want := FileName("my repo", "abc123"), "my_repo-"+hash8("abc123")+".yaml"; got != want {
		t.Errorf("FileName = %q, want %q", got, want)
	}
}

func TestNextCreatesFileAndIncrements(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sequences")
	f := Open(dir, "k", "p", "/repo/p")
	n, err := f.Next("TIK", 0)
	if err != nil || n != 1 {
		t.Fatalf("first Next = %d, %v; want 1", n, err)
	}
	if n, _ = f.Next("TIK", 0); n != 2 {
		t.Fatalf("second Next = %d, want 2", n)
	}
	if n, _ = f.Next("MEM-DB", 0); n != 1 {
		t.Fatalf("other prefix Next = %d, want 1", n)
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName("p", "k")))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lastUsedBy: /repo/p", "TIK: 2", "MEM-DB: 1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("file lacks %q:\n%s", want, data)
		}
	}
}

func TestNextHonoursTheFloor(t *testing.T) {
	f := Open(t.TempDir(), "k", "p", "o")
	if n, _ := f.Next("TIK", 7); n != 8 {
		t.Fatalf("Next with floor 7 = %d, want 8", n)
	}
	if n, _ := f.Next("TIK", 3); n != 9 {
		t.Fatalf("a lower floor must not lower the counter: got %d, want 9", n)
	}
}

func TestCorruptFileIsTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName("p", "k")), []byte(":: not yaml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := Open(dir, "k", "p", "o").Next("TIK", 4); err != nil || n != 5 {
		t.Fatalf("Next over a corrupt file = %d, %v; want 5", n, err)
	}
	if n, _ := Open(dir, "k", "p", "o").Peek("TIK"); n != 5 {
		t.Fatalf("the corrupt file was not rewritten: Peek = %d", n)
	}
}

func TestRaiseNeverLowers(t *testing.T) {
	f := Open(t.TempDir(), "k", "p", "o")
	if err := f.Raise("B", 55); err != nil {
		t.Fatal(err)
	}
	if err := f.Raise("B", 10); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.Peek("B"); n != 55 {
		t.Fatalf("Peek after Raise(55), Raise(10) = %d, want 55", n)
	}
	if n, _ := f.Next("B", 0); n != 56 {
		t.Fatalf("Next after Raise(55) = %d, want 56", n)
	}
}

func TestPeekWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if n, err := Open(dir, "k", "p", "o").Peek("TIK"); err != nil || n != 0 {
		t.Fatalf("Peek on a missing file = %d, %v; want 0", n, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Peek wrote files: %v", entries)
	}
}

func TestTheFileFollowsTheLabelAndKeepsItsCounters(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir, "k", "myproject", "o").Next("B", 203); err != nil {
		t.Fatal(err)
	}
	n, err := Open(dir, "k", "btb", "o").Next("B", 0)
	if err != nil || n != 205 {
		t.Fatalf("Next after the folder rename = %d, %v; want 205", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName("btb", "k"))); err != nil {
		t.Errorf("file not renamed to the new label: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName("myproject", "k"))); !os.IsNotExist(err) {
		t.Errorf("old name still there (stat err %v)", err)
	}
}

func TestPeekFindsTheFileUnderAnotherLabelWithoutRenaming(t *testing.T) {
	dir := t.TempDir()
	if err := Open(dir, "k", "old", "o").Raise("W", 67); err != nil {
		t.Fatal(err)
	}
	if n, _ := Open(dir, "k", "new", "o").Peek("W"); n != 67 {
		t.Errorf("Peek under another label = %d, want 67", n)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName("old", "k"))); err != nil {
		t.Errorf("Peek renamed the file: %v", err)
	}
}

func TestAnOlderCommitNamedFileIsAdopted(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "commit-"+hash8("k")+".yaml")
	if err := os.WriteFile(old, []byte("owner: /repo\ncounters:\n    B: 204\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := Open(dir, "k", "btb", "o").Next("B", 0); err != nil || n != 205 {
		t.Fatalf("Next = %d, %v; want 205 from the older file", n, err)
	}
	for _, gone := range []string{old, old + ".lock"} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s still there (stat err %v)", filepath.Base(gone), err)
		}
	}
}

func TestTwoFilesForOneKeyAreMerged(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		FileName("a", "k"): "counters:\n    TIK: 5\n    B: 9\n",
		FileName("b", "k"): "counters:\n    TIK: 9\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := Open(dir, "k", "c", "o")
	if n, _ := f.Next("TIK", 0); n != 10 {
		t.Errorf("Next(TIK) = %d, want 10 (the larger counter)", n)
	}
	if n, _ := f.Peek("B"); n != 9 {
		t.Errorf("Peek(B) = %d, want 9 (kept from the other file)", n)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*-"+hash8("k")+".yaml"))
	if len(matches) != 1 {
		t.Errorf("files for the key after the merge: %v, want one", matches)
	}
}

func TestPathReportsTheCurrentFile(t *testing.T) {
	dir := t.TempDir()
	f := Open(dir, "k", "p", "o")
	path, exists, err := f.Path()
	if err != nil || exists || path != filepath.Join(dir, FileName("p", "k")) {
		t.Fatalf("Path before any write = %q, %v, %v", path, exists, err)
	}
	if err := Open(dir, "k", "older", "o").Raise("B", 1); err != nil {
		t.Fatal(err)
	}
	path, exists, _ = f.Path()
	if !exists || path != filepath.Join(dir, FileName("older", "k")) {
		t.Errorf("Path = %q, %v; want the existing older-named file", path, exists)
	}
}

func TestConcurrentHandlesNeverShareANumber(t *testing.T) {
	dir := t.TempDir()
	const workers, each = 8, 50
	var mu sync.Mutex
	seen := map[int]bool{}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		label := []string{"a", "b"}[w%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				n, err := Open(dir, "k", label, "o").Next("TIK", 0)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if seen[n] {
					t.Errorf("number %d issued twice", n)
				}
				seen[n] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for n := 1; n <= workers*each; n++ {
		if !seen[n] {
			t.Fatalf("number %d never issued (got %d numbers)", n, len(seen))
		}
	}
}

func TestRelabelRenamesAnExistingFileOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Open(dir, "k", "p", "o").Relabel(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Relabel created files for a project with no counter: %v", entries)
	}
	if err := Open(dir, "k", "old", "o").Raise("B", 204); err != nil {
		t.Fatal(err)
	}
	if err := Open(dir, "k", "new", "o").Relabel(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName("old", "k"))); !os.IsNotExist(err) {
		t.Errorf("old name still there (stat err %v)", err)
	}
	if n, _ := Open(dir, "k", "new", "o").Peek("B"); n != 204 {
		t.Errorf("counter after Relabel = %d, want 204", n)
	}
}
