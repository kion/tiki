package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileKind names one of tiki's own files: its config or its workflow.
type FileKind int

const (
	KindConfig FileKind = iota
	KindWorkflow
)

const (
	ConfigFileName         = "tiki-config.yaml"
	LegacyConfigFileName   = "config.yaml"
	WorkflowFileName       = "tiki-workflow.yaml"
	LegacyWorkflowFileName = "workflow.yaml"
)

// a cloned repo could plant a huge file or a device under a tiki name
const maxTikiFileSize = 1 << 20

// CandidateStatus says what resolution did with one file in a folder.
type CandidateStatus string

const (
	StatusLoaded  CandidateStatus = "loaded"
	StatusIgnored CandidateStatus = "ignored"
	StatusSkipped CandidateStatus = "skipped"
	StatusMissing CandidateStatus = "missing"
)

// Candidate is one file resolution looked at, with its status and the reason for it.
type Candidate struct {
	Path   string
	Status CandidateStatus
	Reason string
}

// FolderResolution is the file one folder contributes for a kind, if any.
type FolderResolution struct {
	Dir        string
	Global     bool
	Chosen     string
	Candidates []Candidate
}

// Resolution is a kind's files across the user config folder and the project folder.
// Local is nil when the project folder is the user config folder.
type Resolution struct {
	Global FolderResolution
	Local  *FolderResolution
}

// Names returns the preferred and the legacy file name for the kind.
func (k FileKind) Names() (preferred, legacy string) {
	if k == KindWorkflow {
		return WorkflowFileName, LegacyWorkflowFileName
	}
	return ConfigFileName, LegacyConfigFileName
}

// ResolveFiles resolves the kind in the user config folder and the project folder.
// Nothing is cached: writers create files after an earlier resolution in the same process.
func ResolveFiles(kind FileKind) Resolution {
	pm := mustGetPathManager()
	r := Resolution{Global: ResolveInFolder(kind, pm.configDir, true)}
	if !sameFolder(pm.configDir, pm.projectRoot) {
		local := ResolveInFolder(kind, pm.projectRoot, false)
		r.Local = &local
	}
	return r
}

// ResolveInFolder picks at most one file of the kind from dir:
// the tiki- name when present, otherwise the legacy name if it is recognised as tiki's.
// The global folder belongs to tiki, so its legacy file is accepted without a check.
func ResolveInFolder(kind FileKind, dir string, global bool) FolderResolution {
	preferred, legacy := kind.Names()
	prefPath := filepath.Join(dir, preferred)
	legacyPath := filepath.Join(dir, legacy)
	r := FolderResolution{Dir: dir, Global: global}

	prefExists := exists(prefPath)
	if prefExists {
		if reason := unsafeReason(prefPath); reason != "" {
			r.Candidates = append(r.Candidates, Candidate{Path: prefPath, Status: StatusSkipped, Reason: reason})
		} else {
			r.Chosen = prefPath
			r.Candidates = append(r.Candidates, Candidate{Path: prefPath, Status: StatusLoaded})
		}
	}

	if exists(legacyPath) {
		switch {
		case prefExists:
			r.Candidates = append(r.Candidates, Candidate{Path: legacyPath, Status: StatusIgnored, Reason: preferred + " takes precedence"})
		default:
			if reason := legacyRejection(kind, legacyPath, global); reason != "" {
				r.Candidates = append(r.Candidates, Candidate{Path: legacyPath, Status: StatusSkipped, Reason: reason})
			} else {
				r.Chosen = legacyPath
				r.Candidates = append(r.Candidates, Candidate{Path: legacyPath, Status: StatusLoaded})
			}
		}
	}

	if len(r.Candidates) == 0 {
		r.Candidates = []Candidate{{Path: prefPath, Status: StatusMissing}}
	}
	return r
}

// IsDeletableTikiFile reports whether reset may delete path.
// Stricter than reading: a local legacy file must parse and pass the key check,
// because templated YAML from other tools is invalid YAML and common.
func IsDeletableTikiFile(kind FileKind, path string, global bool) bool {
	preferred, _ := kind.Names()
	if filepath.Base(path) == preferred || global {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	top, err := topLevelKeys(data)
	if err != nil {
		return false
	}
	return recognitionFailure(kind, top) == ""
}

func legacyRejection(kind FileKind, path string, global bool) string {
	if reason := unsafeReason(path); reason != "" {
		return reason
	}
	if global {
		return ""
	}
	// a broken tiki file should report its read or parse error, not vanish
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	top, err := topLevelKeys(data)
	if err != nil {
		if _, isShape := err.(shapeError); !isShape {
			return ""
		}
		return err.Error()
	}
	return recognitionFailure(kind, top)
}

type shapeError string

func (e shapeError) Error() string { return string(e) }

// topLevelKeys returns the file's top-level keys.
// A shapeError means valid YAML that isn't a non-empty map; any other error is a parse error.
func topLevelKeys(data []byte) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, shapeError("empty")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, shapeError("not a YAML map")
	}
	if len(root.Content) == 0 {
		return nil, shapeError("empty")
	}
	keys := make([]string, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys = append(keys, root.Content[i].Value)
	}
	return keys, nil
}

var workflowMarkerKeys = map[string]bool{
	"fields": true, "views": true, "statuses": true, "types": true,
	"triggers": true, "prefix": true, "prefixseparator": true,
}

func recognitionFailure(kind FileKind, keys []string) string {
	if kind == KindWorkflow {
		for _, k := range keys {
			if workflowMarkerKeys[strings.ToLower(k)] {
				return ""
			}
		}
		return "no tiki workflow keys"
	}
	known := knownConfigKeys()
	var unknown []string
	for _, k := range keys {
		if !known[strings.ToLower(k)] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return ""
	}
	sort.Strings(unknown)
	return fmt.Sprintf("not a tiki file (unknown keys: %s)", strings.Join(unknown, ", "))
}

// knownConfigKeys derives the top-level config keys from Config's tags,
// so a new setting can't be forgotten by the recognition check.
func knownConfigKeys() map[string]bool {
	t := reflect.TypeOf(Config{})
	keys := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		if tag := t.Field(i).Tag.Get("mapstructure"); tag != "" {
			keys[strings.ToLower(tag)] = true
		}
	}
	return keys
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func unsafeReason(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	if !info.Mode().IsRegular() {
		return "not a regular file"
	}
	if info.Size() > maxTikiFileSize {
		return "larger than 1 MiB"
	}
	return ""
}

func sameFolder(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return os.SameFile(ai, bi)
}

// personal settings describe the person running tiki, not the project
var personalConfigKeys = []string{"ai", "identity", "logging"}

// PersonalKeysIn returns the personal settings a config file sets, sorted.
// In a project file they apply to everyone who uses the project.
func PersonalKeysIn(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	top, err := topLevelKeys(data)
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(top))
	for _, k := range top {
		set[strings.ToLower(k)] = true
	}
	var found []string
	for _, k := range personalConfigKeys {
		if set[k] {
			found = append(found, k)
		}
	}
	return found
}

// All returns every candidate, global folder first.
func (r Resolution) All() []Candidate {
	all := append([]Candidate(nil), r.Global.Candidates...)
	if r.Local != nil {
		all = append(all, r.Local.Candidates...)
	}
	return all
}
