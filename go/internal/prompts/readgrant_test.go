package prompts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	_ "github.com/handline/handline/go/agentctl/agent/vendors"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/access"
)

// A read of another agent's memory or of any transcript offers to keep all of them; any other call offers nothing.
func TestAReadOfAMemoryOrATranscriptOffersTheWholeGrant(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	cfg, err := team.AccountConfigDir(e.Home, teamtest.Account(t, e.Home, "main"))
	if err != nil || os.MkdirAll(cfg, 0o755) != nil {
		t.Fatal(err)
	}
	agents := paths.AgentsDir(e.Home)
	want := []string{agents, filepath.Join(cfg, "projects")}
	for _, c := range []struct {
		tool, key, path string
		offers          bool
	}{
		{"Read", "file_path", filepath.Join(agents, "a7", "memory", "MEMORY.md"), true},
		{"Grep", "path", filepath.Join(agents, "a7", "memory"), true},
		{"Glob", "pattern", filepath.Join(agents, "a7", "memory", "*.md"), true},
		{"Read", "file_path", filepath.Join(cfg, "projects", "-wt", "s.jsonl"), true},
		{"Read", "file_path", filepath.Join(agents, "a7", "agent.yaml"), false},
		{"Read", "file_path", filepath.Join(cfg, ".credentials.json"), false},
		{"Read", "file_path", "memory/MEMORY.md", false},
		{"Bash", "command", filepath.Join(agents, "a7", "memory"), false},
	} {
		got := e.ReadGrant(c.tool, map[string]any{c.key: c.path})
		if c.offers && !slices.Equal(got, want) || !c.offers && got != nil {
			t.Errorf("%s %s: %v", c.tool, c.path, got)
		}
	}
}

// Allow always is offered only with a grant. It allows the call and keeps the grant as always; a plain allow keeps nothing.
func TestAllowAlwaysKeepsTheGrantAndAPlainAllowKeepsNothing(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	agent := backendKey(e)
	memory := filepath.Join(paths.AgentsDir(e.Home), "other", "memory", "MEMORY.md")
	askRead := func() (<-chan asked, *Block) {
		out := make(chan asked, 1)
		go func() {
			r, err := e.permissionAsk(asBackend(e), map[string]any{"tool": "Read", "input": map[string]any{"file_path": memory}})
			m, _ := r.(map[string]any)
			out <- asked{m, err}
		}()
		return out, waitOpen(t, e, memory)
	}
	keys := func(p map[string]any) []string {
		var out []string
		for _, o := range p["options"].([]any) {
			out = append(out, o.(map[string]any)["key"].(string))
		}
		return out
	}

	call, b := askRead()
	if k := keys(openPrompt(t, e, agent)); !slices.Equal(k, []string{"allow", "always", "deny"}) {
		t.Fatalf("options %v", k)
	}
	if _, err := answers(e, b.Fingerprint, "allow"); err != nil {
		t.Fatal(err)
	}
	if r := reply(t, call); r["behavior"] != "allow" {
		t.Fatalf("%v", r)
	}
	if gs, _ := access.Grants(e.DB, agent); len(gs) != 0 {
		t.Fatalf("a plain allow kept %v", gs)
	}

	// Allowing always is a grant, which only the owner gives: a member's is refused and leaves the question open.
	call, b = askRead()
	refusal(t, mustErr(answers(e, b.Fingerprint, "always")), "refused")
	if gs, _ := access.Grants(e.DB, agent); len(gs) != 0 {
		t.Fatalf("a member's always kept %v", gs)
	}
	if _, err := e.answer(asOwner(), map[string]any{"agent": agent, "prompt": b.Fingerprint, "choice": "always"}); err != nil {
		t.Fatal(err)
	}
	if r := reply(t, call); r["behavior"] != "allow" {
		t.Fatalf("%v", r)
	}
	gs, _ := access.Grants(e.DB, agent)
	if len(gs) != 1 || gs[0].Path != paths.AgentsDir(e.Home) || gs[0].Scope != "always" {
		t.Fatalf("kept %+v", gs)
	}
	if by, _, _ := db.Meta(e.DB, db.SettingsByMeta(agent)); by != actor.OwnerKey { // a kept grant is the founder's settings change
		t.Fatalf("the grant's change is kept as %q's", by)
	}
	var label string
	e.DB.QueryRow("SELECT answer_label FROM prompt_blocks WHERE id = ?", b.ID).Scan(&label)
	if label != "Allowed always" {
		t.Fatalf("recorded %q", label)
	}

	// A question with no grant takes no always, and offers none.
	call, b = ask(t, e, "make build")
	if k := keys(openPrompt(t, e, agent)); !slices.Equal(k, []string{"allow", "deny"}) {
		t.Fatalf("options %v", k)
	}
	refusal(t, mustErr(answers(e, b.Fingerprint, "always")), "bad_request")
	if _, err := answers(e, b.Fingerprint, "deny"); err != nil {
		t.Fatal(err)
	}
	reply(t, call)
}

func asOwner() *rpc.Conn {
	return &rpc.Conn{Caller: rpc.Caller{Role: "owner", Kind: "person", Info: map[string]any{"kind": "person", "name": "ron"}}}
}
