package tikistore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTakesTimesFromGitHistory(t *testing.T) {
	for _, tc := range []struct {
		name       string
		subdir     string
		viaSymlink bool
	}{
		{"project at repo root", "", false},
		{"project in repo subfolder", "sub", false},
		{"project in non-ASCII subfolder", "Über", false},
		// git reports the resolved repo root,
		// so a key built from it would miss paths under a cwd reached through a symlinked parent.
		{"repo reached through a symlink", "sub", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			runGit(t, repo, "init", "-q")
			runGit(t, repo, "config", "user.email", "test@example.com")
			runGit(t, repo, "config", "user.name", "testuser")

			project := filepath.Join(repo, tc.subdir)
			doc := filepath.Join(project, "a.md")
			writeDoc(t, doc, "AAAAAA", "a")
			runGit(t, repo, "add", "-A")
			runGit(t, repo, "commit", "-q", "-m", "add", "--date", "2026-01-01T10:00:00Z")
			writeDoc(t, doc, "AAAAAA", "a edited")
			runGit(t, repo, "commit", "-q", "-am", "edit", "--date", "2026-02-01T10:00:00Z")

			// A fresh clone or checkout leaves every file with the same, unrelated mtime;
			// an older one here means only git can supply the expected times.
			old := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
			if err := os.Chtimes(doc, old, old); err != nil {
				t.Fatalf("chtimes: %v", err)
			}

			if tc.viaSymlink {
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(repo, link); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				project = filepath.Join(link, tc.subdir)
			}
			t.Chdir(project)
			s, err := NewTikiStore(project)
			if err != nil {
				t.Fatalf("NewTikiStore: %v", err)
			}
			tk := s.GetTiki("AAAAAA")
			if tk == nil {
				t.Fatal("tiki AAAAAA not loaded")
			}
			if want := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC); !tk.CreatedAt().Equal(want) {
				t.Errorf("CreatedAt = %v, want first commit %v", tk.CreatedAt(), want)
			}
			if want := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC); !tk.UpdatedAt().Equal(want) {
				t.Errorf("UpdatedAt = %v, want last commit %v", tk.UpdatedAt(), want)
			}
		})
	}
}
