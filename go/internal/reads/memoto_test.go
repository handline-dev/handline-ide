package reads

import "testing"

// A comment that opens with a person is written to them: the stored mention names them, the older @human names the task's person.
func TestAMemoSaysWhomItIsWrittenTo(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO people(id, display_name, role, created_at) VALUES(2, 'Kim', 'member', 0)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'bob', 'p2', 1)")
	for _, c := range []struct{ body, want string }{
		{"<@p2> OK?", "p2"},
		{"@human OK?", "p2"},
		{"<@bob> look", ""},
		{"see <@p2> later", ""},
		{"plain", ""},
	} {
		got, err := memoTo(e.DB, c.body, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("memoTo(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}
