package reads

import "testing"

func TestWithoutHumanTagDropsTheLeadingMention(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"<@p1> OK?", "OK?"},
		{"@human OK?", "OK?"},
		{"ask <@p1> later", "ask <@p1> later"},
	} {
		if got := withoutHumanTag(c.in); got != c.want {
			t.Errorf("withoutHumanTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
