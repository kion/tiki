package tikistore

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

var forcedBirth = time.Date(2024, 3, 3, 3, 3, 3, 0, time.UTC)

func forceBirthTime(t *testing.T, at time.Time, ok bool) {
	t.Helper()
	prev := birthTimeOf
	birthTimeOf = func(string, os.FileInfo) (time.Time, bool) { return at, ok }
	t.Cleanup(func() { birthTimeOf = prev })
}

func docWithCreatedLine(title, createdLine string) string {
	return "---\nid: AAAAAA\ntitle: " + title + "\n" + createdLine + "\ntype: story\nstatus: inbox\n---\nbody\n"
}

func newStoreIn(t *testing.T, dir string, opts ...Option) *TikiStore {
	t.Helper()
	t.Chdir(dir)
	s, err := NewTikiStore(dir, opts...)
	if err != nil {
		t.Fatalf("NewTikiStore: %v", err)
	}
	return s
}

func TestLoad_StoredCreatedAtForms(t *testing.T) {
	for _, tc := range []struct {
		name       string
		line       string
		want       time.Time
		wantStored bool
	}{
		{"unquoted UTC", "createdAt: 2025-06-01T08:00:00Z", time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC), true},
		{"quoted UTC", `createdAt: "2025-06-01T08:00:00Z"`, time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC), true},
		{"offset", "createdAt: 2025-06-01T10:00:00+02:00", time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC), true},
		{"space separated", "createdAt: 2025-06-01 08:00:00", time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC), true},
		{"date only", "createdAt: 2025-06-01", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), true},
		{"invalid", "createdAt: not a time", forcedBirth, false},
		{"empty", "createdAt:", forcedBirth, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceBirthTime(t, forcedBirth, true)
			dir := t.TempDir()
			writeRaw(t, filepath.Join(dir, "a.md"), docWithCreatedLine("a", tc.line))

			tk := newStoreIn(t, dir).GetTiki("AAAAAA")
			if tk == nil {
				t.Fatal("tiki not loaded")
			}
			if !tk.CreatedAt().Equal(tc.want) {
				t.Errorf("CreatedAt = %v, want %v", tk.CreatedAt(), tc.want)
			}
			if tc.wantStored && tk.CreatedAt().Location() != time.UTC {
				t.Errorf("CreatedAt location = %v, want UTC", tk.CreatedAt().Location())
			}
			if tk.CreatedAtStored() != tc.wantStored {
				t.Errorf("CreatedAtStored = %v, want %v", tk.CreatedAtStored(), tc.wantStored)
			}
		})
	}
}

func TestLoad_StoredCreatedAtWinsOverGit(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "testuser")
	writeRaw(t, filepath.Join(repo, "a.md"), docWithCreatedLine("a", "createdAt: 2025-06-01T08:00:00Z"))
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "add", "--date", "2026-01-01T10:00:00Z")

	tk := newStoreIn(t, repo).GetTiki("AAAAAA")
	if want := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC); !tk.CreatedAt().Equal(want) {
		t.Errorf("CreatedAt = %v, want stored %v", tk.CreatedAt(), want)
	}
}

func TestLoad_FallsBackToBirthTimeThenMtime(t *testing.T) {
	mtime := time.Date(2023, 5, 5, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		birthOK bool
		want    time.Time
	}{
		{"birth time recorded", true, forcedBirth},
		{"no birth time", false, mtime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceBirthTime(t, forcedBirth, tc.birthOK)
			dir := t.TempDir()
			path := filepath.Join(dir, "a.md")
			writeDoc(t, path, "AAAAAA", "a")
			if err := os.Chtimes(path, mtime, mtime); err != nil {
				t.Fatalf("chtimes: %v", err)
			}

			tk := newStoreIn(t, dir).GetTiki("AAAAAA")
			if !tk.CreatedAt().Equal(tc.want) {
				t.Errorf("CreatedAt = %v, want %v", tk.CreatedAt(), tc.want)
			}
			if tk.CreatedAtStored() {
				t.Error("CreatedAtStored = true for a derived value")
			}
		})
	}
}

func TestLoad_CreatedByFallsBackWhenCreatedAtStored(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, filepath.Join(dir, "a.md"), docWithCreatedLine("a", "createdAt: 2025-06-01T08:00:00Z"))

	tk := newStoreIn(t, dir).GetTiki("AAAAAA")
	if v, ok := tk.Get("createdBy"); !ok || v == "" {
		t.Errorf("createdBy = %v (present %v), want the current identity", v, ok)
	}
}

func TestReloadPaths_StoredCreatedAtBeatsCarriedValue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.md")
	writeDoc(t, path, "AAAAAA", "original")
	s, _, _ := newReloadStore(t, root)
	s.GetTiki("AAAAAA").SetCreatedAt(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC))

	writeExternal(t, path, docWithCreatedLine("edited", "createdAt: 2025-06-01T08:00:00Z"))
	mustReload(t, s, path)

	if want := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC); !s.GetTiki("AAAAAA").CreatedAt().Equal(want) {
		t.Errorf("CreatedAt = %v, want the stored %v", s.GetTiki("AAAAAA").CreatedAt(), want)
	}
}

var createdAtLine = regexp.MustCompile(`(?m)^createdAt: \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`)

func TestMarshal_WritesCreatedAtAfterTitleInUTC(t *testing.T) {
	tk := tikipkg.New()
	tk.SetID("ABC123")
	tk.SetTitle("x")
	tk.SetCreatedAt(time.Date(2025, 6, 1, 10, 0, 0, 500_000_000, time.FixedZone("", 2*3600)))

	out, err := marshalTikiFrontmatter(tk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := "id: ABC123\ntitle: x\ncreatedAt: 2025-06-01T08:00:00Z\n"; !strings.HasPrefix(string(out), want) {
		t.Errorf("frontmatter =\n%s\nwant prefix\n%s", out, want)
	}
}

func TestMarshal_OmitsZeroCreatedAt(t *testing.T) {
	tk := tikipkg.New()
	tk.SetID("ABC123")
	tk.SetTitle("x")

	out, err := marshalTikiFrontmatter(tk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "createdAt") {
		t.Errorf("frontmatter has createdAt for a zero time:\n%s", out)
	}
}

func retitle(t *testing.T, s *TikiStore) {
	t.Helper()
	tk := s.GetTiki("AAAAAA").Clone()
	tk.SetTitle("retitled")
	if err := s.UpdateTiki(tk); err != nil {
		t.Fatalf("UpdateTiki: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestSave_NormalizesStoredCreatedAtToUTC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeRaw(t, path, docWithCreatedLine("a", "createdAt: 2025-06-01T10:00:00+02:00"))
	s := newStoreIn(t, dir)

	retitle(t, s)

	if got := strings.Count(readFile(t, path), "createdAt: 2025-06-01T08:00:00Z"); got != 1 {
		t.Errorf("file has %d UTC createdAt lines, want 1:\n%s", got, readFile(t, path))
	}
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	tk := s.GetTiki("AAAAAA")
	if want := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC); !tk.CreatedAt().Equal(want) || !tk.CreatedAtStored() {
		t.Errorf("after reload CreatedAt = %v (stored %v), want %v stored", tk.CreatedAt(), tk.CreatedAtStored(), want)
	}
}

func TestSave_ReplacesInvalidStoredCreatedAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeRaw(t, path, docWithCreatedLine("a", "createdAt: garbage"))
	s := newStoreIn(t, dir)

	retitle(t, s)

	got := readFile(t, path)
	if strings.Contains(got, "garbage") || len(createdAtLine.FindAllString(got, -1)) != 1 {
		t.Errorf("file should hold one valid createdAt line and no garbage:\n%s", got)
	}
}

func TestSave_InMemoryCreatedAtMatchesFile(t *testing.T) {
	forceBirthTime(t, time.Date(2024, 3, 3, 3, 3, 3, 700_000_000, time.FixedZone("", 3600)), true)
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "a.md"), "AAAAAA", "a")
	s := newStoreIn(t, dir)

	retitle(t, s)

	got := s.GetTiki("AAAAAA").CreatedAt()
	if want := time.Date(2024, 3, 3, 2, 3, 3, 0, time.UTC); !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("in-memory CreatedAt = %v, want exactly the written %v", got, want)
	}
}
