package reads

import "testing"

func TestWithoutLeadDropsTheLeadingMention(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"<@p1> OK?", "OK?"},
		{"ask <@p1> later", "ask <@p1> later"},
	} {
		if got := withoutLead(c.in); got != c.want {
			t.Errorf("withoutLead(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
