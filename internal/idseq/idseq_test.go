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

func TestFileName(t *testing.T) {
	sum := sha256.Sum256([]byte("abc123"))
	want := "my_repo-" + hex.EncodeToString(sum[:])[:8] + ".yaml"
	if got := FileName("my repo", "abc123"); got != want {
		t.Errorf("FileName = %q, want %q", got, want)
	}
}

func TestNextCreatesFileAndIncrements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sequences", "p.yaml")
	f := Open(path, "/repo/p")
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
	data, err := os.ReadFile(path)
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
	f := Open(filepath.Join(t.TempDir(), "p.yaml"), "o")
	if n, _ := f.Next("TIK", 7); n != 8 {
		t.Fatalf("Next with floor 7 = %d, want 8", n)
	}
	if n, _ := f.Next("TIK", 3); n != 9 {
		t.Fatalf("a lower floor must not lower the counter: got %d, want 9", n)
	}
}

func TestCorruptFileIsTreatedAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(path, []byte(":: not yaml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	f := Open(path, "o")
	if n, err := f.Next("TIK", 4); err != nil || n != 5 {
		t.Fatalf("Next over a corrupt file = %d, %v; want 5", n, err)
	}
	if n, _ := Open(path, "o").Peek("TIK"); n != 5 {
		t.Fatalf("the corrupt file was not rewritten: Peek = %d", n)
	}
}

func TestRaiseNeverLowers(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "p.yaml"), "o")
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
	path := filepath.Join(t.TempDir(), "p.yaml")
	if n, err := Open(path, "o").Peek("TIK"); err != nil || n != 0 {
		t.Fatalf("Peek on a missing file = %d, %v; want 0", n, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Peek created the file (stat err %v)", err)
	}
}

func TestConcurrentHandlesNeverShareANumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.yaml")
	const workers, each = 8, 50
	var mu sync.Mutex
	seen := map[int]bool{}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				n, err := Open(path, "o").Next("TIK", 0)
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
