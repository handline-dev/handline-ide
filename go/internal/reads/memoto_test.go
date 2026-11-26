package reads

import "testing"

// A comment that opens with a person is written to them; one that opens with an agent, or with no mention, to no one.
func TestAMemoSaysWhomItIsWrittenTo(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ body, want string }{
		{"<@p2> OK?", "p2"},
		{"<@bob> look", ""},
		{"see <@p2> later", ""},
		{"plain", ""},
	} {
		if got := memoTo(c.body); got != c.want {
			t.Errorf("memoTo(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}
