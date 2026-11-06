package verdict

// HungAfterS is how long a turn may run with nothing heard before it counts as hung: the one bound the healer, the reconciler and the
// nudge schedule share, so the screen and the RUNNING label agree on what stuck is.
const HungAfterS = 1800

// Silent is how many minutes an agent reading RUNNING has gone with nothing heard, and whether that makes its turn hung. lastHeard is 0
// when nothing was ever heard. A closed session or one with no terminal is never hung: nothing runs there to be silent.
func Silent(running bool, lastHeard int64, closed, hasTerminal bool, now int64) (mins int64, hung bool) {
	if !running || lastHeard <= 0 || closed || !hasTerminal {
		return 0, false
	}
	silent := now - lastHeard
	return silent / 60, silent >= HungAfterS
}
