package prompts

import (
	"errors"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

func asRon() *rpc.Conn {
	return &rpc.Conn{Caller: rpc.Caller{Role: "member", Info: map[string]any{"kind": "person", "name": "casey"}}}
}

func register(t *testing.T, e *Env) {
	t.Helper()
	teamtest.Set(t, e.Home, "backend", "vendor", "claude")
}

// backendKey is the key of the agent register seeds as backend.
func backendKey(e *Env) string { return paths.WhoKey(e.Home, "backend") }

func answers(e *Env, prompt string, choice any) (any, error) {
	return e.answer(asRon(), map[string]any{"agent": backendKey(e), "prompt": prompt, "choice": choice})
}

func refusal(t *testing.T, err error, code string) *rpc.Error {
	t.Helper()
	var re *rpc.Error
	if !errors.As(err, &re) || re.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return re
}

func mustErr(_ any, err error) error { return err }

func TestAStalePromptGetsTheCurrentOneAndNothingIsDone(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	call, b := ask(t, e, "go vet ./...")
	re := refusal(t, mustErr(answers(e, "not-the-open-one", "allow")), "stale_prompt")
	if re.Data["current"] == nil {
		t.Fatalf("a stale answer is told the current prompt: %v", re.Data)
	}
	if _, err := answers(e, b.Fingerprint, "allow"); err != nil {
		t.Fatal(err)
	}
	reply(t, call)
	re = refusal(t, mustErr(answers(e, b.Fingerprint, "allow")), "stale_prompt")
	if re.Data["current"] != nil {
		t.Fatalf("with no prompt open there is no current one: %v", re.Data)
	}
}

func TestAKeyAHookQuestionDoesNotOfferIsABadRequest(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	call, b := ask(t, e, "go vet ./...")
	for _, choice := range []any{"yes", "no", "esc", "1", "", []any{"allow"}, map[string]any{"text": "x"}, 7} {
		refusal(t, mustErr(answers(e, b.Fingerprint, choice)), "bad_request")
	}
	_, err := e.answer(asRon(), map[string]any{"agent": backendKey(e), "choice": "allow"})
	refusal(t, err, "bad_request")
	_, err = e.answer(asRon(), map[string]any{"agent": "nobody", "prompt": b.Fingerprint, "choice": "allow"})
	refusal(t, err, "not_found")
	if _, err := answers(e, b.Fingerprint, "deny"); err != nil {
		t.Fatal(err)
	}
	reply(t, call)
}
