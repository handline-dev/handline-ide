package daemon

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
)

func startTranscript(t *testing.T, transcripts bool) *Daemon {
	t.Helper()
	dir := t.TempDir()
	pre, err := storetest.OpenAt(t, filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	pre.Close()
	teamtest.Seed(t, dir, "bob")
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), Auth: pidAuth{}, Transcripts: transcripts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func TestHelloSaysTranscriptOnlyWhenTheDaemonServesIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		srvEnd := pipeTo(t, startTranscript(t, on), "k")
		r := srvEnd.ask(`{"jsonrpc":"2.0","id":2,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
		if got := r["result"].(map[string]any)["capabilities"].(map[string]any)["transcript"]; got != on {
			t.Fatalf("serving %v: hello says %v", on, got)
		}
	}
}

func TestHelloSaysFounderTypedWhenTheDaemonTypesTheFoundersWords(t *testing.T) {
	for _, on := range []bool{false, true} {
		d := startTranscript(t, false)
		if on {
			d.env().FounderTyped = func(string) bool { return true }
		}
		r := pipeTo(t, d, "k").ask(`{"jsonrpc":"2.0","id":2,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
		if got := r["result"].(map[string]any)["capabilities"].(map[string]any)["founder_typed"]; got != on {
			t.Fatalf("typing %v: hello says %v", on, got)
		}
	}
}
