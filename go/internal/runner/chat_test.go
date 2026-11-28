package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/kit/paths"
)

// A line the master writes again, its first answer lost with the link, reaches the CLI once; a name outside the chat folder is refused.
func TestAChatLineWrittenTwiceReachesTheCLIOnce(t *testing.T) {
	home, err := os.MkdirTemp("", "rc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	sock := paths.ChatSocket(home, "bob")
	os.MkdirAll(filepath.Dir(sock), 0o700)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go holder.Run(ctx, holder.Config{Argv: []string{"/bin/cat"}, Socket: sock, Linger: time.Second}) // what it is given comes back
	for end := time.Now().Add(5 * time.Second); !holder.Alive(sock); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("no holder")
		}
	}
	r := &Runner{cfg: Config{Home: home}}
	name := filepath.Base(sock)
	if got, err := r.chatList(); err != nil || len(got.(map[string]any)["holders"].([]string)) != 1 {
		t.Fatalf("listed %v %v", got, err)
	}
	for _, line := range []string{`{"n":1}`, `{"n":1}`, `{"n":2}`} {
		if _, err := r.chatWrite(map[string]any{"holder": name, "line": line}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.chatWrite(map[string]any{"holder": "../daemon.sock", "line": `{"n":3}`}); err == nil {
		t.Fatal("a socket outside the chat folder was written")
	}
	rd, err := holder.Open(ctx, sock, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	for _, want := range []string{`{"n":1}`, `{"n":2}`} {
		rec, err := rd.Next()
		if err != nil || string(rec.Line) != want {
			t.Fatalf("read %s %v, want %s", rec.Line, err, want)
		}
	}
}
