package main

import "testing"

// A person's rule lists with its area written as a lesson's text starts, so a merge that copies it keeps its area, and how often it was
// said; an agent's own lesson lists as it is.
func TestALessonLineShowsARulesAreaAndCount(t *testing.T) {
	for _, c := range []struct {
		l    obj
		want string
	}{
		{obj{"id": 1.0, "text": "Use flat rows.", "area": "screen", "count": 18.0, "task": 4.0}, "1  Screen: Use flat rows.  (said 18 times)  (T4)"},
		{obj{"id": 2.0, "text": "by hand", "area": "work", "count": 1.0}, "2  Work: by hand"},
		{obj{"id": 3.0, "text": "Run the tests first.", "count": 1.0}, "3  Run the tests first."},
	} {
		if got := lessonLine(c.l); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
