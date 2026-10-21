package store

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// agentFacingTypeListMarker is the shape every agent-facing observation type
// list uses in this repository.
const agentFacingTypeListMarker = "- **type**: "

// unmappedObservationTypes are the types the store recognises without a
// default review window. The decay map documents them as unmapped, and they
// are the only names an agent-facing list may carry without a review window.
var unmappedObservationTypes = map[string]bool{
	"learning":        true,
	"session_summary": true,
}

// TestAgentFacingTypeListCoversReviewScheduledTypes ties the observation types
// the agent-facing memory protocol offers to the types this store schedules a
// review for.
//
// The store keys `policy` with a twelve-month review window, but the protocol
// the installer writes for the agent listed seven types and never mentioned
// `policy`. An agent following its own instructions could not produce a
// `policy` observation, so that review window was unreachable from the agent
// side, and nothing validated the two lists, so the drift was silent.
//
// The guard finds every agent-facing type list in the repository instead of
// listing them, so a copy added later is covered without touching this test.
func TestAgentFacingTypeListCoversReviewScheduledTypes(t *testing.T) {
	scheduled := make(map[string]bool, len(decayReviewAfterMonths))
	for observationType := range decayReviewAfterMonths {
		scheduled[observationType] = true
	}

	lists := agentFacingTypeLists(t)
	if len(lists) == 0 {
		t.Fatal("no agent-facing observation type list found in the repository")
	}

	for path, found := range lists {
		t.Run(path, func(t *testing.T) {
			for _, types := range found {
				listed := make(map[string]bool, len(types))
				for _, observationType := range types {
					listed[observationType] = true
					if !scheduled[observationType] && !unmappedObservationTypes[observationType] {
						t.Errorf("agent-facing type %q is neither review-scheduled nor a documented unmapped type", observationType)
					}
				}
				for observationType := range scheduled {
					if !listed[observationType] {
						t.Errorf("review-scheduled type %q is missing from the agent-facing type list, so the store's review window for it is unreachable from the agent side", observationType)
					}
				}
			}
		})
	}
}

// agentFacingTypeLists finds every agent-facing observation type list in the
// repository, keyed by the file that carries it.
func agentFacingTypeLists(t *testing.T) map[string][][]string {
	t.Helper()

	lists := map[string][][]string{}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "vendor", ".engram", "tmp":
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".md", ".go", ".ts", ".sh":
		default:
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			index := strings.Index(line, agentFacingTypeListMarker)
			if index < 0 {
				continue
			}
			types := parseAgentFacingTypeList(line[index+len(agentFacingTypeListMarker):])
			if len(types) == 0 {
				continue
			}
			lists[path] = append(lists[path], types)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	return lists
}

// parseAgentFacingTypeList reads the pipe-separated type names. The copies
// spell the names slightly differently, so the markers around each one are
// stripped.
func parseAgentFacingTypeList(list string) []string {
	parts := strings.Split(list, "|")
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), "`")
		if trimmed == "" {
			continue
		}
		types = append(types, trimmed)
	}
	sort.Strings(types)
	return types
}
