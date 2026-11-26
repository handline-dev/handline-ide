package taskwrites

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// contextServer serves the agent context calls over d to as ("" is the founder).
func contextServer(d *sql.DB, home, as string) *rpc.Server {
	s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = d
	RegisterAgentContext(s, home)
	return s
}

// refusedGate is the gate a refused call names, "" for none.
func refusedGate(r map[string]any) string {
	e, _ := r["error"].(map[string]any)
	data, _ := e["data"].(map[string]any)
	g, _ := data["gate"].(string)
	return g
}

// The founder reads an agent's role and standing context with who wrote each, and replaces either as a new version written by them that
// waits for the agent's next fresh start; an agent may not, and empty or unchanged text writes nothing.
func TestTheFounderReadsAndReplacesAnAgentsContext(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker")
	worker := teamtest.Key(t, home, "gp/worker")
	server := func(as string) *rpc.Server { return contextServer(d, home, as) }
	rid := 0
	set := func(as, params string) map[string]any {
		rid++
		return call(t, server(as), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"agent.context.set","params":{%s"request_id":"r%d"}}`, params, rid))[0]
	}
	get := func() map[string]any {
		r := call(t, server(""), fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"agent.context","params":{"agent":%q}}`, worker))[0]
		got, ok := r["result"].(map[string]any)
		if !ok {
			t.Fatalf("agent.context %v", r)
		}
		return got
	}
	if got := get(); got["role"] != nil || got["base"] != nil {
		t.Fatalf("an agent with no texts yet: %v", got)
	}
	if gate := refusedGate(set(worker, fmt.Sprintf(`"agent":%q,"role":"I do anything",`, worker))); gate != "role" {
		t.Errorf("an agent replaced its own role: gate %q", gate)
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,`, worker))); !strings.Contains(got, "role, base or both") {
		t.Errorf("nothing named: %q", got)
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"base":"  ",`, worker))); !strings.Contains(got, "not empty") {
		t.Errorf("empty base: %q", got)
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"role":"a\u0007b",`, worker))); !strings.Contains(got, "control character") {
		t.Errorf("control character: %q", got)
	}
	before, _ := db.LastInstructionID(d, 0)
	r := set("", fmt.Sprintf(`"agent":%q,"role":"Builds the daemon.\n\nSmall pieces.",`, worker))
	if said := fmt.Sprint(r["result"].(map[string]any)["said"]); !strings.Contains(said, "role replaced; applies when it next starts fresh") {
		t.Fatalf("said %q", said)
	}
	role := get()["role"].(map[string]any)
	if role["text"] != "Builds the daemon.\n\nSmall pieces." || role["by"] == "" || role["by"] == worker {
		t.Fatalf("role %v", role)
	}
	in, _ := db.CurrentInstruction(d, worker, db.PartRole)
	if in.Actor != in.Approver || in.ID <= before {
		t.Fatalf("kept %+v", in)
	}
	r = set("", fmt.Sprintf(`"agent":%q,"role":"Builds the daemon.\n\nSmall pieces.\n",`, worker))
	if said := fmt.Sprint(r["result"].(map[string]any)["said"]); !strings.Contains(said, "already") {
		t.Fatalf("the same text again: %q", said)
	}
	if after, _ := db.LastInstructionID(d, 0); after != in.ID {
		t.Fatalf("the same text wrote a version: %d", after)
	}
}

// agent.memory gives the notes in MEMORY.md's order then by file name, with their frontmatter; agent.memory.remove takes away one
// note's file and its index line and leaves every other byte of MEMORY.md as it was; only the founder may.
func TestTheFounderReadsAnAgentsMemoryAndRemovesOneNote(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker")
	worker := teamtest.Key(t, home, "gp/worker")
	dir := filepath.Join(paths.AgentDir(home, worker), "memory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	index := "# Memory\n\n- [Worktree](worktree.md) — where I build\n- [Squash](squash.md) — reset to the merge base\n- [Old](archive/squash.md) — kept elsewhere\n\nKept by hand.\n"
	write("MEMORY.md", index)
	write("worktree.md", "---\nname: my-worktree\ndescription: the folder I build in\nmetadata:\n  type: reference\n---\n\nThe worktree is here.\n")
	write("squash.md", "---\nname: squash\ndescription: reset to the merge base\ntype: feedback\n---\nReset soft.\n")
	write("aside.md", "No frontmatter at all.\n")
	write("twin.md", "---\r\nname: squash\r\ntype: user\r\n---\r\nTwin.\r\n")
	write("notes.txt", "not a note")
	server := func(as string) *rpc.Server { return contextServer(d, home, as) }
	list := func() string {
		r := call(t, server(""), fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"agent.memory","params":{"agent":%q}}`, worker))[0]
		got, ok := r["result"].(map[string]any)
		if !ok {
			t.Fatalf("agent.memory %v", r)
		}
		var out []string
		for _, n := range got["notes"].([]any) {
			n := n.(map[string]any)
			out = append(out, fmt.Sprintf("%s|%s|%s|%s|%q", n["name"], n["file"], n["description"], n["type"], n["body"]))
		}
		return strings.Join(out, "\n")
	}
	want := "my-worktree|worktree.md|the folder I build in|reference|\"The worktree is here.\\n\"\n" +
		"squash|squash.md|reset to the merge base|feedback|\"Reset soft.\\n\"\n" +
		"aside|aside.md|||\"No frontmatter at all.\\n\"\n" +
		"squash|twin.md||user|\"Twin.\\n\""
	if got := list(); got != want {
		t.Fatalf("notes:\n%s\nwant:\n%s", got, want)
	}
	remove := func(as, name, rid string) map[string]any {
		return call(t, server(as), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"agent.memory.remove","params":{"agent":%q,"name":%q,"request_id":%q}}`, worker, name, rid))[0]
	}
	if gate := refusedGate(remove(worker, "squash", "r1")); gate != "role" {
		t.Fatalf("an agent removed a memory note: gate %q", gate)
	}
	if _, err := os.Stat(filepath.Join(dir, "squash.md")); err != nil {
		t.Fatal("a refused remove took the file")
	}
	if got := errText(remove("", "squash", "r2")); !strings.Contains(got, "2 notes are named") {
		t.Fatalf("a name two notes share: %q", got)
	}
	if got := errText(remove("", "twin.md", "r2b")); got != "" {
		t.Fatalf("a note named by its file: %q", got)
	}
	if got := errText(remove("", "nobody", "r2c")); !strings.Contains(got, "no memory note") {
		t.Fatalf("no such note: %q", got)
	}
	if got := errText(remove("", "my-worktree", "r3")); got != "" {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(dir, "worktree.md")); !os.IsNotExist(err) {
		t.Fatalf("the note's file stayed: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if string(b) != "# Memory\n\n- [Squash](squash.md) — reset to the merge base\n- [Old](archive/squash.md) — kept elsewhere\n\nKept by hand.\n" {
		t.Fatalf("MEMORY.md now %q", b)
	}
	if got := errText(remove("", "aside", "r4")); got != "" {
		t.Fatalf("a note MEMORY.md does not list: %q", got)
	}
	b2, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if string(b2) != string(b) {
		t.Fatalf("removing an unlisted note changed MEMORY.md: %q", b2)
	}
	if got := list(); got != "squash|squash.md|reset to the merge base|feedback|\"Reset soft.\\n\"" {
		t.Fatalf("left %s", got)
	}
	if got := refusedGate(call(t, server(worker), fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"agent.memory","params":{"agent":%q}}`, worker))[0]); got != "role" {
		t.Fatalf("an agent read memory notes: gate %q", got)
	}
	if got := errText(call(t, server(""), `{"jsonrpc":"2.0","id":3,"method":"agent.memory","params":{"agent":"a999"}}`)[0]); !strings.Contains(got, "no agent") {
		t.Fatalf("an agent that does not exist: %q", got)
	}
}
