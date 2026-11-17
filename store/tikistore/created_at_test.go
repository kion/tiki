package tikistore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var forcedBirth = time.Date(2024, 3, 3, 3, 3, 3, 0, time.UTC)

func forceBirthTime(t *testing.T, at time.Time, ok bool) {
	t.Helper()
	prev := birthTimeOf
	birthTimeOf = func(string, os.FileInfo) (time.Time, bool) { return at, ok }
	t.Cleanup(func() { birthTimeOf = prev })
}

func docWithCreatedLine(id, title, createdLine string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\n" + createdLine + "\ntype: story\nstatus: inbox\n---\nbody\n"
}

func newStoreIn(t *testing.T, dir string) *TikiStore {
	t.Helper()
	t.Chdir(dir)
	s, err := NewTikiStore(dir)
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
			writeRaw(t, filepath.Join(dir, "a.md"), docWithCreatedLine("AAAAAA", "a", tc.line))

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
	writeRaw(t, filepath.Join(repo, "a.md"), docWithCreatedLine("AAAAAA", "a", "createdAt: 2025-06-01T08:00:00Z"))
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
	writeRaw(t, filepath.Join(dir, "a.md"), docWithCreatedLine("AAAAAA", "a", "createdAt: 2025-06-01T08:00:00Z"))

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

	writeExternal(t, path, docWithCreatedLine("AAAAAA", "edited", "createdAt: 2025-06-01T08:00:00Z"))
	mustReload(t, s, path)

	if want := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC); !s.GetTiki("AAAAAA").CreatedAt().Equal(want) {
		t.Errorf("CreatedAt = %v, want the stored %v", s.GetTiki("AAAAAA").CreatedAt(), want)
	}
}
