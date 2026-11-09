//go:build !windows

package tmux_test

import (
	"testing"

	"github.com/handline/handline/go/agentctl/term/tmux"
)

func TestParsing(t *testing.T) {
	got := tmux.ParsePanes("handline\t0\tmy win\t%7\t42\tzsh\t/a b\t99\t0\nbad line\nhandline\t1\tx\tnot-a-pane\t1\tzsh\t/\t1\t0\n")
	if len(got) != 1 || got[0].WindowName != "my win" || got[0].Path != "/a b" || got[0].PID != 42 || got[0].Activity != 99 || got[0].Dead {
		t.Fatalf("%+v", got)
	}
}
