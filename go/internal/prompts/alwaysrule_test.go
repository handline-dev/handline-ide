package prompts

import (
	"slices"
	"testing"

	"github.com/handline/handline/go/scheduler/access"
)

// askCmd calls permission.ask for a Bash command, and waits until it is the open prompt.
func askCmd(t *testing.T, e *Env, command string) (<-chan asked, *Block) {
	t.Helper()
	out := make(chan asked, 1)
	go func() {
		r, err := e.permissionAsk(asBackend(e), map[string]any{"tool": "Bash", "input": map[string]any{"command": command}})
		m, _ := r.(map[string]any)
		out <- asked{m, err}
	}()
	return out, waitOpen(t, e, command)
}

func optionKeys(p map[string]any) []string {
	var out []string
	for _, o := range p["options"].([]any) {
		out = append(out, o.(map[string]any)["key"].(string))
	}
	return out
}

func TestAlwaysAllowIsOfferedOnlyWhenTheAgentWasAskedTheSameRuleBefore(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	agent := backendKey(e)

	call, b := askCmd(t, e, "npm test")
	if k := optionKeys(openPrompt(t, e, agent)); !slices.Equal(k, []string{"allow", "deny"}) {
		t.Fatalf("a first question keeps two buttons: %v", k)
	}
	refusal(t, mustErr(answers(e, b.Fingerprint, "always")), "bad_request")
	e.DB.Exec("UPDATE prompt_blocks SET detail = json_set(detail, '$.offer_rule', 1) WHERE id = ?", b.ID)
	if _, err := e.answer(asOwner(), map[string]any{"agent": agent, "prompt": b.Fingerprint, "choice": "always"}); err == nil {
		t.Fatal("an offer written into the row is no offer")
	}
	answers(e, b.Fingerprint, "allow")
	if r := reply(t, call); r["behavior"] != "allow" || r["rule"] != nil {
		t.Fatalf("%v", r)
	}

	call, b = askCmd(t, e, "npm test")
	p := openPrompt(t, e, agent)
	opts := p["options"].([]any)
	if k := optionKeys(p); !slices.Equal(k, []string{"allow", "always", "deny"}) {
		t.Fatalf("a repeat offers the rule: %v", k)
	}
	if once, always := opts[0].(map[string]any), opts[1].(map[string]any); once["label"] != "Allow once" ||
		always["label"] != "Always allow <@"+agent+"> Bash(npm test)" || always["rule"] != "Bash(npm test)" {
		t.Fatalf("%v", opts)
	}
	refusal(t, mustErr(answers(e, b.Fingerprint, "always")), "refused") // only the owner keeps a rule
	// The row is in a database agents can write: a rule put in it is not the one kept or applied.
	e.DB.Exec("UPDATE prompt_blocks SET detail = json_set(detail, '$.rule', 'Bash(*)') WHERE id = ?", b.ID)
	if _, err := e.answer(asOwner(), map[string]any{"agent": agent, "prompt": b.Fingerprint, "choice": "always"}); err != nil {
		t.Fatal(err)
	}
	r := reply(t, call)
	if rule, _ := r["rule"].(map[string]any); r["behavior"] != "allow" || rule["tool"] != "Bash" || rule["content"] != "npm test" {
		t.Fatalf("the running session takes the rule: %v", r)
	}
	if rs, _ := access.RulesForLaunch(e.DB, e.Home, agent); !slices.Equal(rs, []string{"Bash(npm test)"}) {
		t.Fatalf("kept %v", rs)
	}
}

func TestOnlyASafeExactBashCommandIsOffered(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if r, ok := e.exactRule("Bash", map[string]any{"command": "npm test"}); !ok || r.String() != "Bash(npm test)" {
		t.Errorf("%v %v", r, ok)
	}
	if _, ok := e.exactRule("Bash", map[string]any{"command": "rm -rf build/*"}); ok {
		t.Error("a * in the command would be a wildcard in its rule")
	}
	if _, ok := e.exactRule("Read", map[string]any{"command": "npm test"}); ok {
		t.Error("only Bash: a read has its own grant")
	}
}
