package nodes

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/handline/handline/go/internal/runner"
)

// A path under the master's home is the same path under the runner's, in every text of an answer; a path that only starts like the
// home, or holds it inside, is left alone. The attachments a text names are found, a sentence's last stop not taken for part of the path.
func TestPathsUnderTheMastersHomeAreTheRunnersAndAttachmentsAreFound(t *testing.T) {
	got := swapAll(map[string]any{"a": "/m/home/agents/k/memory and /m/homework", "b": []any{"/m/home", 3}}, "/m/home", "/r/h")
	want := map[string]any{"a": "/r/h/agents/k/memory and /m/homework", "b": []any{"/r/h", 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	r := &Registry{Home: "/m/home"}
	refs := r.attachmentRefs("see /m/home/attachments/ab12.png. and `/m/home/attachments/cd.pdf`, not /m/home/shared/x")
	if !reflect.DeepEqual(refs, []string{"/m/home/attachments/ab12.png", "/m/home/attachments/cd.pdf"}) {
		t.Fatalf("%q", refs)
	}
	if refs := r.attachmentRefs("/m/home/attachments/../accounts and /m/home/attachments/x/../../db.sqlite"); len(refs) != 0 {
		t.Fatalf("a path out of the attachments was taken for one: %q", refs)
	}
	if got := swap("in /m/home. and /x/m/home/y", "/m/home", "/r/h"); got != "in /r/h. and /x/m/home/y" {
		t.Fatalf("%q", got)
	}
}

// A runner's sessions get the shared MCP config without each server whose program it said it lacks (pencil here), the rest as written. A
// program it was not asked about yet keeps its server, as does a server at a URL; nothing dropped leaves the file as it is.
func TestARunnerGetsTheSharedMCPServersWhoseProgramItHas(t *testing.T) {
	config := []byte(`{"mcpServers":{"pencil":{"command":"/opt/pencil/bin/pencil","args":["mcp"]},` +
		`"git":{"command":"git","env":{"A":"1"}},"web":{"url":"https://x.example/mcp?a=1&b=2"}},"other":true}`)
	r := &Registry{beats: map[string]heard{}}
	says := func(asked, has []string) {
		r.beats["n1"] = heard{beat: runner.Beat{Has: runner.Has{Asked: asked, Programs: has}}}
	}
	says([]string{"git", "pencil"}, []string{"git"})
	var got map[string]any
	if err := json.Unmarshal(r.serversThere("n1", config), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"mcpServers": map[string]any{"git": map[string]any{"command": "git", "env": map[string]any{"A": "1"}},
		"web": map[string]any{"url": "https://x.example/mcp?a=1&b=2"}}, "other": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	for _, c := range []struct{ asked, has []string }{{[]string{"git", "pencil"}, []string{"git", "pencil"}}, {[]string{"git"}, []string{"git"}}, {nil, nil}} {
		says(c.asked, c.has)
		if string(r.serversThere("n1", config)) != string(config) {
			t.Fatalf("asked %v, has %v: the config changed", c.asked, c.has)
		}
	}
	if string(r.serversThere("n2", config)) != string(config) {
		t.Fatal("a runner not heard from had its config changed")
	}
}
