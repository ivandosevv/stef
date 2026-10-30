// Package watcher observes one explicit root; it never follows symlinks.
package watcher

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

const MaxFileSize = 256 * 1024
const MaxFiles = 10000

var skipped = map[string]bool{".git": true, ".stef": true, "node_modules": true, "dist": true, "build": true, "bin": true, "vendor": true, ".venv": true, "__pycache__": true, ".svelte-kit": true, ".next": true, "coverage": true}

type Watcher struct {
	root  string
	fs    *fsnotify.Watcher
	hub   *hub.Hub
	files map[string]map[string]Symbol
	git   bool
}

func New(root string, h *hub.Hub) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{root: root, fs: fw, hub: h, files: make(map[string]map[string]Symbol)}
	cmd := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree")
	w.git = cmd.Run() == nil
	if err := w.scan(root, false); err != nil {
		fw.Close()
		return nil, fmt.Errorf("scan repository: %w", err)
	}
	return w, nil
}

func (w *Watcher) Close() error { return w.fs.Close() }

func (w *Watcher) relative(name string) (string, bool) {
	rel, err := filepath.Rel(w.root, name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	for _, part := range strings.Split(rel, "/") {
		if skipped[part] {
			return "", false
		}
	}
	return rel, true
}

func (w *Watcher) gitIgnored(rel string) bool {
	if !w.git || rel == "." {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", w.root, "check-ignore", "-q", "--", rel)
	return cmd.Run() == nil
}

func (w *Watcher) read(name string) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFileSize {
		return nil, nil
	}
	rel, ok := w.relative(name)
	if !ok {
		return nil, nil
	}
	// Root-confined open also prevents an ancestor symlink swap from escaping.
	file, err := os.OpenInRoot(w.root, rel)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > MaxFileSize || !utf8.Valid(content) || strings.ContainsRune(string(content), 0) {
		return nil, nil
	}
	return content, nil
}

func (w *Watcher) scan(root string, emit bool) error {
	return w.scanWithCapture(root, emit, true)
}

func (w *Watcher) scanWithCapture(root string, emit, capture bool) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, ok := w.relative(name)
		if !ok {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if w.gitIgnored(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return w.fs.Add(name)
		}
		if len(w.files) >= MaxFiles {
			return fmt.Errorf("repository exceeds %d tracked files; choose a smaller root", MaxFiles)
		}
		if emit {
			return w.processWithCapture(name, capture)
		}
		data, err := w.read(name)
		if err != nil {
			return err
		}
		if data != nil {
			w.files[rel] = Extract(rel, data)
		}
		return nil
	})
}

func (w *Watcher) Run(ctx context.Context) error {
	type captureState struct {
		enabled bool
		epoch   uint64
	}
	pending := make(map[string]captureState)
	ticker := time.NewTicker(180 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.fs.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher: %v", err)
		case event, ok := <-w.fs.Events:
			if !ok {
				return nil
			}
			if _, ok := w.relative(event.Name); !ok {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
				mode, epoch := w.hub.CaptureState()
				state := captureState{enabled: mode == telemetry.UpdatesFiles, epoch: epoch}
				if previous, exists := pending[event.Name]; exists && (!previous.enabled || previous.epoch != epoch) {
					// A coalesced save across a mode change belongs to the suppressed
					// baseline; do not revive it because a later fs event arrived.
					state.enabled = false
				}
				pending[event.Name] = state
			}
		case <-ticker.C:
			names := make([]string, 0, len(pending))
			for name := range pending {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				mode, epoch := w.hub.CaptureState()
				queued := pending[name]
				capture := queued.enabled && mode == telemetry.UpdatesFiles && queued.epoch == epoch
				info, err := os.Lstat(name)
				if err == nil && info.IsDir() {
					if err := w.scanWithCapture(name, true, capture); err != nil {
						log.Printf("watch directory: %v", err)
					}
				} else if err := w.processWithCapture(name, capture); err != nil {
					log.Printf("watch file: %v", err)
				}
				delete(pending, name)
			}
		}
	}
}

func (w *Watcher) process(name string) error {
	return w.processWithCapture(name, true)
}

func (w *Watcher) processWithCapture(name string, capture bool) error {
	rel, ok := w.relative(name)
	if !ok || w.gitIgnored(rel) {
		return nil
	}
	content, err := w.read(name)
	if os.IsNotExist(err) {
		// Removing a whole directory must remove its recorded descendants too.
		names := []string{}
		for file := range w.files {
			if file == rel || strings.HasPrefix(file, rel+"/") {
				names = append(names, file)
			}
		}
		sort.Strings(names)
		for _, file := range names {
			if err := w.changes(file, w.files[file], nil, capture); err != nil {
				return err
			}
			delete(w.files, file)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if content == nil {
		return nil
	}
	if _, exists := w.files[rel]; !exists && len(w.files) >= MaxFiles {
		return fmt.Errorf("tracked file limit reached")
	}
	current := Extract(rel, content)
	if err := w.changes(rel, w.files[rel], current, capture); err != nil {
		return err
	}
	w.files[rel] = current
	return nil
}

func (w *Watcher) changes(file string, before, after map[string]Symbol, capture bool) error {
	// Still advance the baseline in process, without building suppressed diffs.
	if !capture || w.hub.Settings().UpdateMode != telemetry.UpdatesFiles {
		return nil
	}
	nodes := make(map[string]struct{})
	for node := range before {
		nodes[node] = struct{}{}
	}
	for node := range after {
		nodes[node] = struct{}{}
	}
	keys := make([]string, 0, len(nodes))
	for node := range nodes {
		keys = append(keys, node)
	}
	sort.Strings(keys)
	for _, node := range keys {
		a, old := before[node]
		b, new := after[node]
		if old && new && a.Code == b.Code {
			continue
		}
		action := "modified"
		if !old {
			action = "created"
		}
		if !new {
			action = "deleted"
			b.Kind = a.Kind
			b.Line = a.Line
		}
		event := telemetry.Event{AgentID: "filesystem", NodeName: node, Action: action, Summary: strings.Title(b.Kind) + " " + action, Why: "Observed a file change; no agent rationale was supplied.", Status: "ready", Source: "watcher", Kind: b.Kind, Line: b.Line, Language: strings.TrimPrefix(filepath.Ext(file), "."), Before: Clip(a.Code), After: Clip(b.Code), Diff: Diff(node, a.Code, b.Code)}
		if _, err := w.hub.Publish(event); err != nil {
			return err
		}
	}
	return nil
}
