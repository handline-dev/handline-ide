package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// offerOf is an offer of version v whose files hold data, and a read that serves them in pieces of 3 bytes.
func offerOf(v string, data map[string]string) (BuildOffer, func(string, int64) ([]byte, error)) {
	o := BuildOffer{Version: v}
	for _, n := range BuildNames {
		sum := sha256.Sum256([]byte(data[n]))
		o.Files = append(o.Files, BuildFile{Name: n, Size: int64(len(data[n])), Sum: hex.EncodeToString(sum[:])})
	}
	return o, func(name string, off int64) ([]byte, error) {
		return []byte(data[name][off:min(off+3, int64(len(data[name])))]), nil
	}
}

// The master's build is kept once each file's sha256 is the one named, the newest replacing the one before in place; a file whose
// sha256 is not is refused, says why, and leaves the kept build as it was.
func TestTheMastersBuildIsKeptOnlyWhenItsSumsMatch(t *testing.T) {
	home := t.TempDir()
	o, read := offerOf("v1", map[string]string{"handlined": "daemon one", "hl": "cli one"})
	if err := fetchBuild(home, o, read); err != nil {
		t.Fatal(err)
	}
	kept := KeptBuild(home, "v1")
	if b, _ := os.ReadFile(kept); string(b) != "daemon one" || KeptBuild(home, "v2") != "" {
		t.Fatalf("kept %q: %q", kept, b)
	}
	if fi, err := os.Stat(filepath.Join(BuildDir(home), "hl")); err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("hl: %v %v", fi, err)
	}
	o, read = offerOf("v2", map[string]string{"handlined": "daemon two", "hl": "cli two"})
	o.Files[1].Sum = strings.Repeat("0", 64)
	err := fetchBuild(home, o, read)
	if err == nil || !strings.Contains(err.Error(), "hl: its sha256 is") || !strings.Contains(err.Error(), "not the 0000") {
		t.Fatalf("a wrong sum: %v", err)
	}
	if KeptBuild(home, "v1") == "" || KeptBuild(home, "v2") != "" {
		t.Fatal("a refused build replaced the kept one")
	}
	if _, err := os.Stat(BuildDir(home) + ".part"); !os.IsNotExist(err) {
		t.Fatalf("the part folder stayed: %v", err)
	}
	o, read = offerOf("v2", map[string]string{"handlined": "daemon two", "hl": "cli two"})
	if err := fetchBuild(home, o, read); err != nil || KeptBuild(home, "v2") == "" || KeptBuild(home, "v1") != "" {
		t.Fatalf("the newest: %v", err)
	}
}

// A session the runner holds goes on through its start again on the master's build: the new process takes it from runner.json, its
// terminal not closed, and its heartbeat names it.
func TestASessionGoesOnThroughTheStartAgain(t *testing.T) {
	home := t.TempDir()
	tm, tok := &fakeTerm{}, &fakeTokens{kept: map[string]int{}}
	r, err := Start(Config{Home: home, Term: tm, Tokens: tok, Auth: fakeAuth{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.serve("runner.terminal.open", map[string]any{"agent": "bob", "folder": home, "command": "claude"}); err != nil {
		t.Fatal(err)
	}
	r.Close()
	again, err := Start(Config{Home: home, Term: tm, Tokens: tok, Auth: fakeAuth{}})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	sessions, _ := again.Beat()["sessions"].([]any)
	if len(sessions) != 1 || fmt.Sprint(sessions[0]) != "map[agent:bob handle:%1]" || len(tm.closed) != 0 {
		t.Fatalf("after the start again: %+v, closed %v", again.Beat()["sessions"], tm.closed)
	}
}
