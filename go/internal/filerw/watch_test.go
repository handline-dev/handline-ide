package filerw

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// watching is a rig whose loaded docs are looked at every 20 ms.
func watching(t *testing.T) *rig {
	r := newRig(t)
	r.env.WatchEvery = 20 * time.Millisecond
	return r
}

// hookEdit is a PostToolUse hook's object for tool on path.
func hookEdit(tool, key, path string) map[string]any {
	return map[string]any{"tool_name": tool, "tool_input": map[string]any{key: path}}
}

func TestAChangeToTheFileIsTakenInWithoutAPushAndNamesTheAgentThatMadeIt(t *testing.T) {
	r := watching(t)
	r.put("a.md", "one\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	r.env.Touched("bob", hookEdit("Edit", "file_path", filepath.Join(r.wt, "a.md")))
	r.put("a.md", "one\ntwo\n")
	var got []map[string]any
	waitFor(t, "the disk update", func() bool { got = e.told("doc_changed"); return len(got) == 1 })
	u := got[0]["updates"].([]any)[0].(map[string]any)
	if u["client"] != "disk" || u["by"] != "bob" || got[0]["doc"] != id {
		t.Fatalf("%v", got[0])
	}
	if res := result(t, e.pull("a.md", id, 0)); res["version"] != 1.0 {
		t.Fatalf("pull %v", res)
	}
	time.Sleep(100 * time.Millisecond) // more looks: the change is taken in once
	if len(e.told("doc_changed")) != 1 {
		t.Fatalf("taken in again: %v", e.told("doc_changed"))
	}
}

func TestAChangeNoEditNamedIsTakenInAtTheNextLookWithNoOneNamed(t *testing.T) {
	r := watching(t)
	r.put("a.md", "one\n")
	e := r.client()
	e.open("a.md")
	r.put("a.md", "two\n")
	var got []map[string]any
	waitFor(t, "the disk update", func() bool { got = e.told("doc_changed"); return len(got) == 1 })
	if u := got[0]["updates"].([]any)[0].(map[string]any); u["by"] != nil {
		t.Fatalf("named %v", u["by"])
	}
}

func TestAFileDeletedWhileTheDocIsIdleLetsItGo(t *testing.T) {
	r := watching(t)
	r.put("a.md", "x")
	e := r.client()
	id, _, _ := e.open("a.md")
	os.Remove(filepath.Join(r.wt, "a.md"))
	waitFor(t, "doc_closed", func() bool { return len(e.told("doc_closed")) == 1 })
	if got := e.told("doc_closed")[0]; got["reason"] != "gone" || got["doc"] != id {
		t.Fatalf("%v", got)
	}
}

func TestOnlyAnEditToolsAbsolutePathNamesTheAgentForTenSeconds(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newRig(t)
	r.env.Now = func() time.Time { return now }
	r.put("a.md", "x")
	full := filepath.Join(r.wt, "a.md")
	r.env.Touched("bob", hookEdit("Read", "file_path", full))
	r.env.Touched("bob", hookEdit("Edit", "file_path", "a.md"))
	if got := r.env.touchedBy(realPath(full), false); got != "" {
		t.Fatalf("a read or a relative path named %q", got)
	}
	r.env.Touched("bob", hookEdit("NotebookEdit", "notebook_path", full))
	now = now.Add(touchKeep)
	if got := r.env.touchedBy(realPath(full), false); got != "bob" {
		t.Fatalf("within ten seconds: %q", got)
	}
	now = now.Add(time.Second)
	if got := r.env.touchedBy(realPath(full), false); got != "" {
		t.Fatalf("after ten seconds: %q", got)
	}
}

// loaded is the doc r's env holds for rel.
func (r *rig) loaded(rel string) *doc {
	r.env.docs.mu.Lock()
	defer r.env.docs.mu.Unlock()
	return r.env.docs.all[docKey("alice", rel)]
}

func TestAChangeWaitsOneLookForTheEditThatNamesIt(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "one\n")
	e := r.client()
	e.open("a.md")
	d := r.loaded("a.md")
	r.put("a.md", "one\ntwo\n")
	r.env.look(d)
	if len(e.told("doc_changed")) != 0 {
		t.Fatal("taken in before the edit's hook could name it")
	}
	r.env.Touched("bob", hookEdit("Write", "file_path", filepath.Join(r.wt, "a.md")))
	r.env.look(d)
	var got []map[string]any
	waitFor(t, "the disk update", func() bool { got = e.told("doc_changed"); return len(got) == 1 })
	if u := got[0]["updates"].([]any)[0].(map[string]any); u["by"] != "bob" {
		t.Fatalf("by %v", u["by"])
	}
}

func TestAFileThatAlreadyHoldsTheDocsTextIsNotTakenIn(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "a\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	result(t, e.push("a.md", id, 0, cs(2, []any{0, "b", ""})))
	r.put("a.md", "a\nb\n") // another view saved the same text
	d := r.loaded("a.md")
	r.env.Touched("bob", hookEdit("Write", "file_path", filepath.Join(r.wt, "a.md")))
	r.env.look(d)
	time.Sleep(50 * time.Millisecond)
	if got := e.told("doc_changed"); len(got) != 1 {
		t.Fatalf("taken in: %v", got)
	}
	e.close("a.md")
	if r.file("a.md") != "a\nb\n" {
		t.Fatalf("file %q", r.file("a.md"))
	}
}

func TestAnEditNamesOneChangeOnly(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "one\n")
	e := r.client()
	e.open("a.md")
	d := r.loaded("a.md")
	r.env.Touched("bob", hookEdit("MultiEdit", "file_path", filepath.Join(r.wt, "a.md")))
	r.put("a.md", "one\ntwo\n")
	r.env.look(d)
	os.WriteFile(filepath.Join(r.wt, "a.md"), []byte("one\ntwo\nthree\n"), 0o644) // a git checkout, say
	r.env.look(d)
	r.env.look(d)
	var got []map[string]any
	waitFor(t, "two disk updates", func() bool { got = e.told("doc_changed"); return len(got) == 2 })
	by := func(i int) any { return got[i]["updates"].([]any)[0].(map[string]any)["by"] }
	if by(0) != "bob" || by(1) != nil {
		t.Fatalf("by %v then %v", by(0), by(1))
	}
}

func TestAWaitForTheEditEndsWhenTheDocsOwnWriteIsSeen(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "a\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	d := r.loaded("a.md")
	r.put("a.md", "a\nb\n") // a change no edit names
	r.env.look(d)           // waits a look
	result(t, e.push("a.md", id, 0, cs(2, []any{0, "c", ""})))
	r.env.flushLater(d)           // the write takes the change in and writes
	r.put("a.md", "a\nb\nc\nd\n") // bob's write, whose hook comes after the next look
	r.env.look(d)
	r.env.Touched("bob", hookEdit("Write", "file_path", filepath.Join(r.wt, "a.md")))
	r.env.look(d)
	var got []map[string]any
	waitFor(t, "bob's update", func() bool { got = e.told("doc_changed"); return len(got) == 3 })
	if u := got[2]["updates"].([]any)[0].(map[string]any); u["by"] != "bob" {
		t.Fatalf("by %v: an old wait took bob's change in before its hook could name it", u["by"])
	}
}

func TestALookCarriesAChangeOverPushesNotYetWritten(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "one\ntwo\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	d := r.loaded("a.md")
	result(t, e.push("a.md", id, 0, cs([]any{0, "1 "}, 8)))
	r.env.Touched("bob", hookEdit("Edit", "file_path", filepath.Join(r.wt, "a.md")))
	r.put("a.md", "one\nTWO\n")
	r.env.look(d)
	if _, _, text := e.open("a.md"); text != "1 one\nTWO\n" {
		t.Fatalf("text %q", text)
	}
	e.close("a.md")
	if r.file("a.md") != "1 one\nTWO\n" {
		t.Fatalf("file %q", r.file("a.md"))
	}
}

func TestADocIsLetGoWhenItsFileTurnsUnreadableOrItsWorktreeGoes(t *testing.T) {
	r := watching(t)
	r.put("a.md", "x")
	r.put("b.md", "x")
	e := r.client()
	e.open("a.md")
	os.WriteFile(filepath.Join(r.wt, "a.md"), []byte{0xff, 0xfe}, 0o644)
	waitFor(t, "doc_closed", func() bool { return len(e.told("doc_closed")) == 1 })
	if got := e.told("doc_closed")[0]["reason"]; got != "unreadable" {
		t.Fatalf("reason %v", got)
	}
	e.open("b.md")
	if _, err := r.env.DB.Exec("UPDATE agent_state SET worktree = NULL WHERE agent = 'alice'"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "doc_closed for the worktree", func() bool { return len(e.told("doc_closed")) == 2 })
	if got := e.told("doc_closed")[1]["reason"]; got != "gone" {
		t.Fatalf("reason %v", got)
	}
}

func TestAFilePutInPlaceWithTheSameTimeAndSizeIsSeen(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "a\n")
	full := filepath.Join(r.wt, "a.md")
	fi, _ := os.Stat(full)
	e := r.client()
	e.open("a.md")
	d := r.loaded("a.md")
	r.put("b.tmp", "b\n")
	os.Chtimes(filepath.Join(r.wt, "b.tmp"), fi.ModTime(), fi.ModTime())
	os.Rename(filepath.Join(r.wt, "b.tmp"), full) // an atomic save
	r.env.look(d)
	r.env.look(d)
	waitFor(t, "the disk update", func() bool { return len(e.told("doc_changed")) == 1 })
}

func TestAFileWhoseTimeAloneChangedIsNotTakenIn(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "a\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	result(t, e.push("a.md", id, 0, cs(2, []any{0, "b"}))) // not written yet
	d := r.loaded("a.md")
	later := time.Now().Add(time.Minute)
	os.Chtimes(filepath.Join(r.wt, "a.md"), later, later)
	r.env.look(d)
	r.env.look(d)
	time.Sleep(50 * time.Millisecond)
	if got := e.told("doc_changed"); len(got) != 1 {
		t.Fatalf("taken in: %v", got)
	}
}
