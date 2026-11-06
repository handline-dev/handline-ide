package verdict

import "testing"

func TestStuckKindParsesStoredWordsAndFoldsWhoAnswers(t *testing.T) {
	for _, c := range []struct {
		word   string
		want   StuckKind
		person bool
	}{
		{"", StuckNone, false},
		{"trust", StuckTrust, true},
		{"dialog", StuckDialog, true},
		{"send_dialog", StuckSendDialog, true},
		{"start", StuckStart, false},
		{"wake", StuckWake, false},
		{"login", StuckLogin, false},
		{"hung", StuckNone, false},
		{"Trust", StuckNone, false},
	} {
		got := ParseStuckKind(c.word)
		if got != c.want || got.NeedsPerson() != c.person {
			t.Errorf("%q: %q person %v", c.word, got, got.NeedsPerson())
		}
		if got != StuckNone && string(got) != c.word {
			t.Errorf("%q does not round-trip: %q", c.word, got)
		}
	}
}
