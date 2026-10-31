package hub

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
)

var workspaceAdjectives = [...]string{"Quiet", "Bright", "Calm", "Clever", "Cozy", "Gentle", "Happy", "Kind", "Lucky", "Mellow", "Nimble", "Patient", "Playful", "Quick", "Rustic", "Shiny", "Silver", "Sunny", "Swift", "Warm", "Wild", "Wise", "Witty", "Zesty"}
var workspaceAnimals = [...]string{"Fox", "Badger", "Bear", "Bison", "Crane", "Deer", "Dolphin", "Falcon", "Finch", "Gecko", "Heron", "Koala", "Lynx", "Moose", "Otter", "Owl", "Panda", "Penguin", "Puffin", "Rabbit", "Robin", "Seal", "Sparrow", "Wren"}
var workspaceNamePattern = regexp.MustCompile(`^[A-Za-z]+ [A-Za-z]+ [0-9]{4,}$`)

// Names are local labels. Randomness and collision checks never call a model or
// replace the stable IDs used to route saved histories.
func generateWorkspaceName(reader io.Reader, used map[string]bool) (string, error) {
	const numbers = 9000
	limit := big.NewInt(int64(len(workspaceAdjectives) * len(workspaceAnimals) * numbers))
	for attempt := 0; attempt < 64; attempt++ {
		value, err := rand.Int(reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate workspace name: %w", err)
		}
		n := int(value.Int64())
		name := fmt.Sprintf("%s %s %d", workspaceAdjectives[n/numbers/len(workspaceAnimals)], workspaceAnimals[n/numbers%len(workspaceAnimals)], n%numbers+1000)
		if !used[strings.ToLower(name)] {
			return name, nil
		}
	}
	// A repeated random result must not cause an infinite retry or duplicate label.
	for number := 1000; ; number++ {
		name := fmt.Sprintf("%s %s %d", workspaceAdjectives[0], workspaceAnimals[0], number)
		if !used[strings.ToLower(name)] {
			return name, nil
		}
	}
}

func (h *Hub) newWorkspaceNameLocked() (string, error) {
	used := make(map[string]bool, len(h.workspaceNames))
	for _, name := range h.workspaceNames {
		used[strings.ToLower(name)] = true
	}
	return generateWorkspaceName(rand.Reader, used)
}

func (h *Hub) replayWorkspaceNameLocked(id, name string) error {
	if name == "" {
		return nil // Older journals receive names after the complete replay.
	}
	if !workspaceNamePattern.MatchString(name) || len(name) > 128 {
		return fmt.Errorf("invalid workspace name journal record")
	}
	if previous := h.workspaceNames[id]; previous != "" && previous != name {
		return fmt.Errorf("workspace name changed in journal")
	}
	for other, saved := range h.workspaceNames {
		if other != id && strings.EqualFold(saved, name) {
			return fmt.Errorf("duplicate workspace name in journal")
		}
	}
	h.workspaceNames[id] = name
	return nil
}

func (h *Hub) ensureWorkspaceNameLocked(id string) error {
	if err := h.admitWorkspaceLocked(id); err != nil {
		return err
	}
	if h.workspaceNames[id] != "" {
		return nil
	}
	name, err := h.newWorkspaceNameLocked()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(sessionControl{Control: "workspace_name", WorkspaceID: id, WorkspaceName: name})
	if err := h.persist(data); err != nil {
		return err
	}
	h.workspaceNames[id] = name
	return nil
}

// Migration appends labels only after replay and recovery. It never rewrites
// history, capture IDs, settings, or memberships.
func (h *Hub) migrateWorkspaceNamesLocked() error {
	if err := h.admitWorkspaceLocked(); err != nil {
		return err
	}
	ids := make([]string, 0, len(h.sessions))
	for id := range h.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := h.ensureWorkspaceNameLocked(id); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hub) sharedWorkspaceLocked(input string) (string, error) {
	input = strings.TrimSpace(input)
	if h.shared[input] {
		return input, nil
	}
	match := ""
	for id, name := range h.workspaceNames {
		if strings.EqualFold(name, input) {
			if match != "" {
				return "", fmt.Errorf("workspace name is ambiguous; use its sharing ID")
			}
			match = id
		}
	}
	if match == "" || !h.shared[match] {
		return "", fmt.Errorf("join requires an existing shared workspace name or ID")
	}
	return match, nil
}
