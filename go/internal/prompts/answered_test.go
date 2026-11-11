package prompts

import (
	"database/sql"
	"testing"
)

// answered is what prompt.answer kept on the block the caller named: label, note and who answered.
func answered(t *testing.T, e *Env, fingerprint string) (label, note, by string) {
	t.Helper()
	var l, n, b sql.NullString
	err := e.DB.QueryRow("SELECT answer_label, answer_note, answer_by FROM prompt_blocks WHERE fingerprint = ?", fingerprint).Scan(&l, &n, &b)
	if err != nil || !l.Valid {
		t.Fatalf("no answer kept for %s: %v", fingerprint, err)
	}
	return l.String, n.String, b.String
}

// What prompt.answer answered is kept for the chat line: a permission's Allowed or Denied with the deny's reason, and who answered.
func TestAnAnswerIsKeptWithItsWordsAndWhoGaveIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	_, b := ask(t, e, "go vet ./...")
	if _, err := answers(e, b.Fingerprint, "allow"); err != nil {
		t.Fatal(err)
	}
	if label, note, by := answered(t, e, b.Fingerprint); label != "Allowed" || note != "" || by != "casey" {
		t.Fatalf("permission allow: %q %q %q", label, note, by)
	}
	_, b = ask(t, e, "rm -rf build")
	if _, err := e.answer(asRon(), map[string]any{"agent": backendKey(e), "prompt": b.Fingerprint, "choice": "deny", "note": " use the script "}); err != nil {
		t.Fatal(err)
	}
	if label, note, _ := answered(t, e, b.Fingerprint); label != "Denied" || note != "use the script" {
		t.Fatalf("permission deny: %q %q", label, note)
	}
}

// A refused answer keeps nothing: the chat line must not say a prompt got an answer it did not.
func TestARefusedAnswerKeepsNothing(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	call, b := ask(t, e, "go vet ./...")
	if _, err := answers(e, "not-the-open-one", "allow"); err == nil {
		t.Fatal("a stale answer was accepted")
	}
	var n int
	if e.DB.QueryRow("SELECT count(*) FROM prompt_blocks WHERE answer_label IS NOT NULL").Scan(&n); n != 0 {
		t.Fatalf("%d answers kept for a refused one", n)
	}
	if _, err := answers(e, b.Fingerprint, "deny"); err != nil {
		t.Fatal(err)
	}
	reply(t, call)
}
