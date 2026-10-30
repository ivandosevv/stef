package hub_test

import (
	"fmt"
	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func event(agent, node, status string) telemetry.Event {
	return telemetry.Event{AgentID: agent, NodeName: node, Action: "modified", Summary: "Changed the request boundary", Why: "This keeps request validation explicit.", Status: status}
}
func newHub(t *testing.T) *hub.Hub {
	t.Helper()
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestOrderedStreamAndCollisionRelease(t *testing.T) {
	h := newHub(t)
	messages, unsubscribe := h.Subscribe()
	defer unsubscribe()
	if snapshot := <-messages; snapshot.Type != "snapshot" || len(snapshot.Events) != 0 {
		t.Fatal("missing initial snapshot")
	}
	first, err := h.Publish(event("alpha", "src/main.go#Run", "running"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "1" || len(first.Collisions) != 0 {
		t.Fatalf("first: %+v", first)
	}
	second, err := h.Publish(event("beta", "src/main.go#Stop", "running"))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Collisions) != 1 || second.Collisions[0] != "alpha" {
		t.Fatalf("file-wide collision missing: %+v", second)
	}
	if _, err := h.Publish(event("alpha", "src/main.go#Run", "ready")); err != nil {
		t.Fatal(err)
	}
	fourth, err := h.Publish(event("beta", "src/main.go#Stop", "running"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fourth.Collisions) != 0 {
		t.Fatal("finished agent still holds claim")
	}
	for i := 1; i <= 4; i++ {
		message := <-messages
		if message.Event.ID != fmt.Sprint(i) {
			t.Fatal("out-of-order stream")
		}
	}
}

func TestConcurrentPublishersAndSlowClient(t *testing.T) {
	h := newHub(t)
	messages, unsubscribe := h.Subscribe()
	defer unsubscribe()
	<-messages
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := event(fmt.Sprint(i), "main.go", "ready")
			if _, err := h.Publish(e); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	snapshot := h.Snapshot()
	if len(snapshot.Events) != 200 {
		t.Fatalf("lost events: %d", len(snapshot.Events))
	}
	for i, e := range snapshot.Events {
		if e.ID != fmt.Sprint(i+1) {
			t.Fatal("nonmonotonic event sequence")
		}
	}
	drained := 0
	for range messages {
		drained++
	}
	if drained != 128 {
		t.Fatalf("slow client buffered %d events", drained)
	}
}

func TestPersistenceRecoversInterruptedJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(event("alpha", "main.go", "ready")); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{\"id\":\"2\""); err != nil {
		t.Fatal(err)
	}
	file.Close()
	h, err = hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := h.Publish(event("beta", "main.go", "ready"))
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != "2" {
		t.Fatal("sequence not recovered")
	}
	h.Close()
	h, err = hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if len(h.Snapshot().Events) != 2 {
		t.Fatal("journal remains corrupted")
	}
}

func TestJournalWithoutFinalNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(event("alpha", "main.go", "ready")); err != nil {
		t.Fatal(err)
	}
	h.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data[:len(data)-1], 0600); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Publish(event("beta", "main.go", "ready")); err != nil {
		t.Fatal(err)
	}
	h.Close()
	h, err = hub.New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if len(h.Snapshot().Events) != 2 {
		t.Fatal("valid last record was joined to the next record")
	}
}
