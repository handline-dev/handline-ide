package turns

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestASubagentsTranscriptIsReadFromItsStartUnderItsToolCall(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s1.jsonl")
	sub := filepath.Join(dir, "s1", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	tr := &Tracker{retryWait: 10 * time.Millisecond, SubLines: func(agent, session, parent string) func([]byte) {
		return func(line []byte) {
			mu.Lock()
			got = append(got, agent+"|"+session+"|"+parent+"|"+string(line))
			mu.Unlock()
		}
	}}
	stop := tr.watchSubagents("bob", "s1", main)
	defer stop()
	os.WriteFile(filepath.Join(sub, "agent-x.jsonl"), []byte("one\ntwo\n"), 0o644)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	early := len(got)
	mu.Unlock()
	if early != 0 {
		t.Fatal("read before its meta named its call")
	}
	os.WriteFile(filepath.Join(sub, "agent-x.meta.json"), []byte(`{"toolUseId":"tu7"}`), 0o644)
	f, _ := os.OpenFile(filepath.Join(sub, "agent-x.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("three\n")
	f.Close()
	for i := 0; i < 200; i++ {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[0] != "bob|s1|tu7|one" || got[2] != "bob|s1|tu7|three" {
		t.Fatal(got)
	}
}

func TestASecondTurnTellsOnlyWhatIsNewInASubagentsTranscript(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "s1", "subagents")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "agent-x.meta.json"), []byte(`{"toolUseId":"tu7"}`), 0o644)
	os.WriteFile(filepath.Join(sub, "agent-x.jsonl"), []byte("one\n"), 0o644)
	var mu sync.Mutex
	var got []string
	tr := &Tracker{retryWait: 10 * time.Millisecond, SubLines: func(_, _, _ string) func([]byte) {
		return func(line []byte) { mu.Lock(); got = append(got, string(line)); mu.Unlock() }
	}}
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(got) }
	stop := tr.watchSubagents("bob", "s1", filepath.Join(dir, "s1.jsonl"))
	for i := 0; i < 100 && count() < 1; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	f, _ := os.OpenFile(filepath.Join(sub, "agent-x.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("two\n") // written between turns
	f.Close()
	stop = tr.watchSubagents("bob", "s1", filepath.Join(dir, "s1.jsonl"))
	for i := 0; i < 100 && count() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	stop()
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "one,two" {
		t.Fatal(got)
	}
}
