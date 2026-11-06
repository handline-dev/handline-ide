package verdict

import "testing"

func TestSilent(t *testing.T) {
	const now = 100_000
	for name, c := range map[string]struct {
		running, closed, terminal bool
		last                      int64
		mins                      int64
		hung                      bool
	}{
		"running, silent past the bound": {true, false, true, now - HungAfterS, HungAfterS / 60, true},
		"running, one second short":      {true, false, true, now - HungAfterS + 1, (HungAfterS - 1) / 60, false},
		"running, heard just now":        {true, false, true, now, 0, false},
		"not running":                    {false, false, true, now - 2*HungAfterS, 0, false},
		"nothing ever heard":             {true, false, true, 0, 0, false},
		"closed session":                 {true, true, true, now - 2*HungAfterS, 0, false},
		"no terminal":                    {true, false, false, now - 2*HungAfterS, 0, false},
		"closed and no terminal":         {true, true, false, now - 2*HungAfterS, 0, false},
	} {
		mins, hung := Silent(c.running, c.last, c.closed, c.terminal, now)
		if mins != c.mins || hung != c.hung {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", name, mins, hung, c.mins, c.hung)
		}
	}
}
