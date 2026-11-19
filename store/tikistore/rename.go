package tikistore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/boolean-maybe/ruki/idfmt"
	"github.com/boolean-maybe/tiki/document"
	"github.com/boolean-maybe/tiki/internal/livestatus"
	"github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

// Rename gives the tiki with id Old the id New.
type Rename struct{ Old, New string }

// MissingPrefix names a tracked tiki's prefix fields that have no value.
type MissingPrefix struct {
	ID     string
	Fields []string
}

// ErrNoProjectPrefix means the workflow has no top-level prefix to migrate to.
var ErrNoProjectPrefix = errors.New("the workflow has no top-level prefix; add one to workflow.yaml first (for example prefix: TIK)")

// CheckRename reports why oldID can't become newID, or nil.
func (s *TikiStore) CheckRename(oldID, newID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.checkRenameLocked(normalizeTikiID(oldID), normalizeTikiID(newID))
}

func (s *TikiStore) checkRenameLocked(oldID, newID string) error {
	tk, ok := s.tikis[oldID]
	if !ok {
		return fmt.Errorf("no tiki with id %s", oldID)
	}
	if !idfmt.IsValidID(newID) {
		return fmt.Errorf("%s is not a valid id (expected %d uppercase letters or digits, or a prefixed id such as ABC-12, ABC_12 or ABC12)", newID, idfmt.IDLength)
	}
	if newID == oldID {
		return fmt.Errorf("%s already has that id", oldID)
	}
	if _, taken := s.tikis[newID]; taken {
		return fmt.Errorf("%s is already taken", newID)
	}
	for id, other := range s.tikis {
		// a tiki may take back its own former id: old references meant it
		if id == oldID {
			continue
		}
		for _, former := range formerIDs(other) {
			if normalizeTikiID(former) == newID {
				return fmt.Errorf("%s is a former id of %s; reusing it would make old references point to a different tiki", newID, id)
			}
		}
	}
	if workflow.PrefixedIDsEnabled() && !idfmt.IsLegacyID(newID) {
		if _, _, ok := workflow.SplitSchemeID(newID); !ok {
			return fmt.Errorf("%s doesn't use this workflow's prefix separator %q (for example %s)",
				newID, workflow.PrefixSeparator(), workflow.FormatID(fieldPrefix(tk), 1))
		}
	}
	if !workflow.MatchesIDScheme(oldID) && workflow.MatchesIDScheme(newID) {
		if missing := missingPrefixFields(tk); len(missing) > 0 {
			return fmt.Errorf("%s has no value in %s, and prefix fields can't be filled once the id is prefixed: set it first, for example %s",
				oldID, strings.Join(missing, ", "), FillExample(oldID, missing[0]))
		}
	}
	if prefix, _, ok := workflow.SplitSchemeID(newID); ok && workflow.PrefixedIDsEnabled() {
		if want := fieldPrefix(tk); prefix != want {
			return fmt.Errorf("%s's fields give the prefix %s, so its new id must start with %s-", oldID, want, want)
		}
	}
	return nil
}

// missingPrefixFields skips notes-only tikis, which have no workflow field at all:
// they can't carry prefix values, so they just take the top-level prefix.
func missingPrefixFields(tk *tiki.Tiki) []string {
	tracked := false
	for _, f := range workflow.WorkflowFields() {
		if tk.Has(f.Name) {
			tracked = true
			break
		}
	}
	if !tracked {
		return nil
	}
	var missing []string
	for _, f := range workflow.PrefixFields() {
		if v, _, _ := tk.StringField(f.Name); v == "" {
			missing = append(missing, f.Name)
		}
	}
	return missing
}

// FillExample is a ready-to-run command that sets field on id, using the field's first value.
func FillExample(id, field string) string {
	value := "<value>"
	if f, ok := workflow.Field(field); ok && len(f.EnumValues) > 0 {
		value = f.EnumValues[0].Value
	}
	return fmt.Sprintf(`tiki exec 'update where id = "%s" set %s="%s"'`, id, field, value)
}

func fieldPrefix(tk *tiki.Tiki) string {
	return workflow.IDPrefixFor(func(field string) string {
		v, _, _ := tk.StringField(field)
		return v
	})
}

// PlanMigration proposes a prefixed id for every tiki that still has a legacy id,
// numbering oldest first so numbers follow creation order. It writes nothing.
// missing lists tracked tikis whose prefix fields are empty:
// those fields freeze once the id is prefixed, so callers refuse to migrate until they're filled.
func (s *TikiStore) PlanMigration() (renames []Rename, missing []MissingPrefix, err error) {
	if !workflow.PrefixedIDsEnabled() {
		return nil, nil, ErrNoProjectPrefix
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var legacy []*tiki.Tiki
	for id, tk := range s.tikis {
		// a scheme id can be legacy-shaped without a separator (B12345), and is already migrated
		if idfmt.IsLegacyID(id) && !workflow.MatchesIDScheme(id) {
			legacy = append(legacy, tk)
		}
	}
	sort.Slice(legacy, func(i, j int) bool {
		a, b := legacy[i], legacy[j]
		if !a.CreatedAt().Equal(b.CreatedAt()) {
			return a.CreatedAt().Before(b.CreatedAt())
		}
		return a.ID() < b.ID()
	})

	last := map[string]int{}
	for _, tk := range legacy {
		prefix := fieldPrefix(tk)
		if _, seen := last[prefix]; !seen {
			alloc, err := s.allocatorLocked()
			if err != nil {
				return nil, nil, err
			}
			stored, err := alloc.Peek(prefix)
			if err != nil {
				return nil, nil, err
			}
			last[prefix] = max(stored, s.highestNumberLocked(prefix))
		}
		last[prefix]++
		renames = append(renames, Rename{Old: tk.ID(), New: workflow.FormatID(prefix, last[prefix])})

		if fields := missingPrefixFields(tk); len(fields) > 0 {
			missing = append(missing, MissingPrefix{ID: tk.ID(), Fields: fields})
		}
	}
	return renames, missing, nil
}

// ApplyRenames renames tikis and rewrites every reference to them:
// id-list and ref fields, and Markdown links in every scanned file,
// wiki pages without an id included.
// Plain-text mentions stay.
// Each renamed tiki keeps its old id in formerIds.
// It returns the files it changed, or with dryRun the files it would change.
// No triggers fire: a rename changes no field a trigger could guard.
func (s *TikiStore) ApplyRenames(renames []Rename, dryRun bool) ([]string, error) {
	changed, err := s.applyRenamesLocked(renames, dryRun)
	if err == nil && !dryRun && len(changed) > 0 {
		s.notifyListeners()
	}
	return changed, err
}

type pendingTiki struct {
	oldID   string
	tk      *tiki.Tiki
	path    string
	content []byte
}

type pendingPage struct {
	path    string
	content string
	mode    os.FileMode
}

type stagedFile struct {
	tmp, target string
}

func (s *TikiStore) applyRenamesLocked(renames []Rename, dryRun bool) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mapping := make(map[string]string, len(renames))
	targets := make(map[string]bool, len(renames))
	for _, r := range renames {
		oldID, newID := normalizeTikiID(r.Old), normalizeTikiID(r.New)
		if err := s.checkRenameLocked(oldID, newID); err != nil {
			return nil, err
		}
		if _, dup := mapping[oldID]; dup {
			return nil, fmt.Errorf("%s is renamed twice", oldID)
		}
		if targets[newID] {
			return nil, fmt.Errorf("%s is the new id of two tikis", newID)
		}
		mapping[oldID] = newID
		targets[newID] = true
	}
	refs := s.referenceMappingLocked(mapping)

	var tikis []pendingTiki
	tikiPaths := make(map[string]bool, len(s.tikis))
	for id, tk := range s.tikis {
		tikiPaths[absPath(tk.Path())] = true
		c := tk.Clone()
		if renameInTiki(c, mapping, refs) {
			tikis = append(tikis, pendingTiki{oldID: id, tk: c})
		}
	}
	pages, err := s.pendingPages(tikiPaths, refs)
	if err != nil {
		return nil, err
	}

	var changed []string
	for _, p := range tikis {
		changed = append(changed, absPath(p.tk.Path()))
	}
	for _, p := range pages {
		changed = append(changed, p.path)
	}
	sort.Strings(changed)
	if dryRun {
		return changed, nil
	}

	// every file is checked and rendered before any is written,
	// so a read-only file or a conflicting edit stops the run with nothing changed
	for i := range tikis {
		path, content, err := s.prepareTikiSave(tikis[i].tk)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tikis[i].tk.Path(), err)
		}
		tikis[i].path, tikis[i].content = path, content
		if err := checkWritable(path); err != nil {
			return nil, err
		}
	}
	for _, p := range pages {
		if err := checkWritable(p.path); err != nil {
			return nil, err
		}
	}
	// raised before writing: a gap is harmless, a number issued twice is not
	for _, newID := range mapping {
		if prefix, n, ok := workflow.SplitSchemeID(newID); ok {
			alloc, err := s.allocatorLocked()
			if err != nil {
				return nil, err
			}
			if err := alloc.Raise(prefix, n); err != nil {
				return nil, err
			}
		}
	}

	var staged []stagedFile
	discard := func() {
		for _, f := range staged {
			_ = os.Remove(f.tmp)
		}
	}
	for _, p := range tikis {
		tmp, err := stageFile(p.path, p.content, 0o644)
		if err != nil {
			discard()
			return nil, err
		}
		staged = append(staged, stagedFile{tmp: tmp, target: p.path})
	}
	for _, p := range pages {
		tmp, err := stageFile(p.path, []byte(p.content), p.mode)
		if err != nil {
			discard()
			return nil, err
		}
		staged = append(staged, stagedFile{tmp: tmp, target: p.path})
	}
	for i, f := range staged {
		if err := os.Rename(f.tmp, f.target); err != nil {
			for _, rest := range staged[i:] {
				_ = os.Remove(rest.tmp)
			}
			return nil, fmt.Errorf("replace %s: %w (files before it in the list were already updated; run the command again to finish)", f.target, err)
		}
	}

	for _, p := range tikis {
		s.finishTikiSave(p.tk, p.path)
		delete(s.tikis, p.oldID)
		s.tikis[p.tk.ID()] = p.tk
	}
	for oldID, newID := range mapping {
		if err := moveLiveStatus(s.dir, oldID, newID); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

// referenceMappingLocked extends the renames with every tiki's former ids,
// so references a failed or hand-made rename left behind follow on the next run.
func (s *TikiStore) referenceMappingLocked(mapping map[string]string) map[string]string {
	refs := make(map[string]string, len(mapping))
	for oldID, newID := range mapping {
		refs[oldID] = newID
	}
	for id, tk := range s.tikis {
		current := id
		if renamed, ok := mapping[id]; ok {
			current = renamed
		}
		for _, former := range formerIDs(tk) {
			former = normalizeTikiID(former)
			if _, live := s.tikis[former]; live {
				continue
			}
			if _, set := refs[former]; !set {
				refs[former] = current
			}
		}
	}
	return refs
}

func checkWritable(path string) error {
	//nolint:gosec // G304: a Markdown file under the scan root
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("can't write %s: %w", path, err)
	}
	return f.Close()
}

func stageFile(target string, data []byte, mode os.FileMode) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	return tmp.Name(), nil
}

func renameInTiki(c *tiki.Tiki, mapping, refs map[string]string) bool {
	changed := false
	if newID, ok := mapping[c.ID()]; ok {
		oldID := c.ID()
		formers := []string{}
		for _, f := range formerIDs(c) {
			if normalizeTikiID(f) != newID {
				formers = append(formers, f)
			}
		}
		c.Set(formerIDsField, append(formers, oldID))
		c.SetID(newID)
		// creation time is matched to git history by id, which is about to change,
		// so a guessed time is written down now rather than re-guessed later
		if c.CreatedAtSource() == tiki.CreatedAtGuess {
			c.SetCreatedAtSource(tiki.CreatedAtFileTimes)
		}
		changed = true
	}
	for _, f := range workflow.WorkflowFields() {
		switch f.Type {
		case workflow.TypeListRef:
			if ids, ok := stringList(c, f.Name); ok {
				if mapped, did := mapIDs(ids, refs); did {
					c.Set(f.Name, mapped)
					changed = true
				}
			}
		case workflow.TypeRef, workflow.TypeID:
			if v, present, _ := c.StringField(f.Name); present {
				if newID, ok := refs[normalizeTikiID(v)]; ok {
					c.Set(f.Name, newID)
					changed = true
				}
			}
		}
	}
	if body := rewriteLinks(c.Body(), refs); body != c.Body() {
		c.SetBody(body)
		changed = true
	}
	return changed
}

func stringList(tk *tiki.Tiki, name string) ([]string, bool) {
	v, ok := tk.Get(name)
	if !ok {
		return nil, false
	}
	switch list := v.(type) {
	case []string:
		return list, true
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, e := range list {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	}
	return nil, false
}

func mapIDs(ids []string, mapping map[string]string) ([]string, bool) {
	out := make([]string, len(ids))
	did := false
	for i, id := range ids {
		out[i] = id
		if newID, ok := mapping[normalizeTikiID(id)]; ok {
			out[i] = newID
			did = true
		}
	}
	return out, did
}

// The renderer resolves [[ID]] only in uppercase, but a link destination in any case.
var (
	wikilinkRefPattern = regexp.MustCompile(`\[\[([A-Z0-9][A-Z0-9_-]*)\]\]`)
	// the label is left out of inline-link matching: it may wrap lines or hold brackets
	idLabelPattern    = regexp.MustCompile(`\[([A-Za-z0-9][A-Za-z0-9_-]*)\](\([ \t]*<?)([A-Za-z0-9][A-Za-z0-9_-]*)`)
	inlineDestPattern = regexp.MustCompile(`\]\(([ \t]*)(<?)([A-Za-z0-9][A-Za-z0-9_-]*)(>?)((?:[ \t]+(?:"[^"\n]*"|'[^'\n]*'|\([^)\n]*\)))?[ \t]*\))`)
	refDefPattern     = regexp.MustCompile(`(?m)^([ ]{0,3}\[[^\]\n]+\]:[ \t]*)(<?)([A-Za-z0-9][A-Za-z0-9_-]*)(>?)([ \t].*)?$`)
)

func rewriteLinks(text string, mapping map[string]string) string {
	text = wikilinkRefPattern.ReplaceAllStringFunc(text, func(m string) string {
		if newID, ok := mapping[m[2:len(m)-2]]; ok {
			return "[[" + newID + "]]"
		}
		return m
	})
	text = idLabelPattern.ReplaceAllStringFunc(text, func(m string) string {
		p := idLabelPattern.FindStringSubmatch(m)
		label, opening, dest := p[1], p[2], p[3]
		// a label that just repeats the id is part of the link, not prose
		if newID, ok := mapping[strings.ToUpper(dest)]; ok && strings.EqualFold(label, dest) {
			return "[" + newID + "]" + opening + dest
		}
		return m
	})
	text = inlineDestPattern.ReplaceAllStringFunc(text, func(m string) string {
		p := inlineDestPattern.FindStringSubmatch(m)
		space, open, dest, closing, rest := p[1], p[2], p[3], p[4], p[5]
		newID, ok := mapping[strings.ToUpper(dest)]
		if !ok || (open == "<") != (closing == ">") {
			return m
		}
		return "](" + space + open + newID + closing + rest
	})
	return refDefPattern.ReplaceAllStringFunc(text, func(m string) string {
		p := refDefPattern.FindStringSubmatch(m)
		head, open, dest, closing, rest := p[1], p[2], p[3], p[4], p[5]
		newID, ok := mapping[strings.ToUpper(dest)]
		if !ok || (open == "<") != (closing == ">") {
			return m
		}
		return head + open + newID + closing + rest
	})
}

// pendingPages finds the scanned Markdown files that aren't loaded tikis
// (wiki pages, or files the loader rejected) whose links need rewriting.
func (s *TikiStore) pendingPages(tikiPaths map[string]bool, mapping map[string]string) ([]pendingPage, error) {
	paths, err := document.WalkDocuments(s.dir)
	if err != nil {
		return nil, fmt.Errorf("scan markdown files: %w", err)
	}
	var pages []pendingPage
	for _, path := range paths {
		path = absPath(path)
		if tikiPaths[path] {
			continue
		}
		//nolint:gosec // G304: a Markdown file under the scan root
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		rewritten := rewriteLinks(string(data), mapping)
		if rewritten == string(data) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		pages = append(pages, pendingPage{path: path, content: rewritten, mode: info.Mode().Perm()})
	}
	return pages, nil
}

func moveLiveStatus(root, oldID, newID string) error {
	st, ok, err := livestatus.Read(root, oldID)
	if err != nil || !ok {
		return err
	}
	if err := livestatus.Write(root, newID, st); err != nil {
		return err
	}
	return livestatus.Remove(root, oldID)
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
