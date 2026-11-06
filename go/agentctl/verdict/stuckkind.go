package verdict

// StuckKind is which stall the daemon raised on an agent. It is stored beside the stalled row, so nothing reads it out of the row's words.
type StuckKind string

const (
	StuckNone       StuckKind = ""
	StuckTrust      StuckKind = "trust"       // the vendor sits at its folder-trust screen
	StuckDialog     StuckKind = "dialog"      // a start-up dialog only a person answers
	StuckSendDialog StuckKind = "send_dialog" // a dialog came up while it worked, so messages cannot reach it
	StuckStart      StuckKind = "start"       // starts for waiting work failed
	StuckWake       StuckKind = "wake"        // wakes started no session
	StuckLogin      StuckKind = "login"       // its account cannot be used and no other can take it; the lift ends it when the account does
)

var stuckKinds = map[StuckKind]bool{StuckTrust: true, StuckDialog: true, StuckSendDialog: true, StuckStart: true, StuckWake: true, StuckLogin: true}

// ParseStuckKind is the kind a stored word names, StuckNone for none or a word it does not know.
func ParseStuckKind(s string) StuckKind {
	if k := StuckKind(s); stuckKinds[k] {
		return k
	}
	return StuckNone
}

// NeedsPerson is whether the stall waits on a person's answer in the terminal, as against one the daemon retries or gives up on.
func (k StuckKind) NeedsPerson() bool {
	return k == StuckTrust || k == StuckDialog || k == StuckSendDialog
}
