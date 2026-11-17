package adminwrites

import (
	"fmt"
	"slices"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/scheduler/worktreesetup"
)

func TestRepoSetKeepsWhatIsNotNamedAndClearsNull(t *testing.T) {
	r := newRig(t)
	repo := t.TempDir()
	teamtest.Set(t, r.home, "alice", "repo", repo)
	st := protected.New(r.home)

	r.result(r.one("repo.set", fmt.Sprintf(`{"path":%q,"script":" pnpm install ","copy":[".env"," config/*.local"],"request_id":"s1"}`, repo)))
	if got := worktreesetup.Get(st, repo); got.Script != "pnpm install" || !slices.Equal(got.Copy, []string{".env", "config/*.local"}) {
		t.Fatalf("set: %+v", got)
	}
	r.result(r.one("repo.set", fmt.Sprintf(`{"path":%q,"script":null,"request_id":"s2"}`, repo)))
	if got := worktreesetup.Get(st, repo); got.Script != "" || len(got.Copy) != 2 {
		t.Fatalf("script cleared, copy kept: %+v", got)
	}
	r.result(r.one("repo.set", fmt.Sprintf(`{"path":%q,"copy":[],"request_id":"s3"}`, repo)))
	if _, there := st.Get(worktreesetup.NS, worktreesetup.Key(repo)); there {
		t.Fatal("an empty setup is kept")
	}
}

func TestRepoSetRefusals(t *testing.T) {
	r := newRig(t)
	repo := t.TempDir()
	teamtest.Set(t, r.home, "alice", "repo", repo)
	for i, c := range []struct{ params, code, field string }{
		{fmt.Sprintf(`{"path":%q,"script":"x"}`, t.TempDir()), "not_found", ""},
		{fmt.Sprintf(`{"path":%q}`, repo), "bad_request", "script"},
		{fmt.Sprintf(`{"path":%q,"script":3}`, repo), "bad_request", "script"},
		{fmt.Sprintf(`{"path":%q,"copy":"x"}`, repo), "bad_request", "copy"},
		{fmt.Sprintf(`{"path":%q,"copy":["../x"]}`, repo), "bad_request", "copy"},
		{fmt.Sprintf(`{"path":%q,"copy":[".git/hooks/pre-commit"]}`, repo), "bad_request", "copy"},
	} {
		params := c.params[:len(c.params)-1] + fmt.Sprintf(`,"request_id":"r%d"}`, i)
		m := r.one("repo.set", params)
		if got := r.code(m); got != c.code {
			t.Errorf("%s: %s, want %s (%v)", c.params, got, c.code, m)
			continue
		}
		if c.field != "" {
			if f := m["error"].(map[string]any)["data"].(map[string]any)["field"]; f != c.field {
				t.Errorf("%s: field %v, want %s", c.params, f, c.field)
			}
		}
	}
	if !worktreesetup.Get(protected.New(r.home), repo).Empty() {
		t.Fatal("a refused call wrote")
	}
}
