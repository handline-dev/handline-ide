package cronspec

import (
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

func TestScheduleValueErrors(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	for _, c := range []struct{ value, want string }{
		{"not json", "schedules is not JSON: Expecting value: line 1 column 1 (char 0)"},
		{`{"cron": "* * * * *"}`, "schedules is a list of {cron, message}"},
		{`[{"cron": "* * * * *", "message": "m"}, {"cron": "* * * * *"}]`, "schedule 2 is {cron, message} and nothing else"},
		{`[{"cron": "x * * * *", "message": "m"}]`, "minute: 'x' is not a number, a range or *"},
		{`[{"cron": "60 * * * *", "message": "m"}]`, "minute: '60' is outside 0-59"},
		{`[{"cron": "*/0 * * * *", "message": "m"}]`, "minute: a step is a positive number, not '0'"},
		{`[{"cron": "* * * *", "message": "m"}]`, "a cron expression has five fields: minute hour day-of-month month day-of-week"},
		{`[{"cron": "* * * * *", "message": "  "}]`, "schedule 1 has no message"},
	} {
		teamtest.Set(t, home, "a", "schedules", c.value)
		_, err := load(home, "a")
		if bv, ok := err.(badValue); !ok || string(bv) != c.want {
			t.Errorf("%s: got %v, want %q", c.value, err, c.want)
		}
	}
}
