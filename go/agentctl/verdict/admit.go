package verdict

// Account is what Admit reads of one account for one agent: each field is a fact the usage package worked out, a "why" string
// being "" when nothing stands against the account on that count.
type Account struct {
	Removing   bool   // the account is being removed
	Excluded   bool   // the caller set it aside
	ExcludeWhy string // and why
	NotFor     string // why no agent of this vendor is ever placed on it (another vendor's, unreadable, its cycle ended)

	// Assignable and AssignWhy are whether a reading lets new work go here (status Assignable, or TakesWork for a stale one).
	Assignable bool
	AssignWhy  string
	NoReading  bool   // no status file at all: no session has ever run on it
	NoRoom     string // why it cannot take one more agent, on the fill the pick ranks it by
	Wall       string // a reading that says it is at its 5-hour or weekly wall, stale readings included while their window lasts
	NoRoomFit  string // why it cannot take one more agent, judged on a fresh fill only (a stale one is unknown, not against it)
}

// Admission is Admit's answer. Best: it may start and the pick ranks it. NeverRun: it has never run, so nothing can rank it, and
// it is the first fallback. Fits: nothing known is against it, the second fallback (Fit). Reasons are everything that stands
// against it, in the order the rules ask; empty when every tier admits it.
type Admission struct {
	Best, NeverRun, Fits bool
	Reasons              []string
}

// Outright is whether the account is refused before any reading is looked at: being removed, set aside, or not for this vendor.
func (a Account) Outright() bool { return a.Removing || a.Excluded || a.NotFor != "" }

// OK is whether any tier admits the account.
func (a Admission) OK() bool { return a.Best || a.NeverRun || a.Fits }

// Admit is the one rule set for whether an agent's session may start on an account. Being removed, set aside or another vendor's
// refuses it outright. Otherwise the pick needs a reading that admits work and room on its fill. A never-run account has no
// reading, which is not a reason against it: the session that starts there writes the first one. Fits asks only what is known:
// a missing or stale reading does not count against it, a reading at the wall or out of room does.
func Admit(a Account) Admission {
	switch {
	case a.Removing:
		return Admission{Reasons: []string{"being removed"}}
	case a.Excluded:
		return Admission{Reasons: []string{a.ExcludeWhy}}
	case a.NotFor != "":
		return Admission{Reasons: []string{a.NotFor}}
	}
	var out Admission
	switch {
	case !a.Assignable && a.NoReading:
		out.NeverRun = true
		out.Reasons = append(out.Reasons, "no reading yet - a session has never run on it")
	case !a.Assignable:
		out.Reasons = append(out.Reasons, a.AssignWhy)
	case a.NoRoom != "":
		out.Reasons = append(out.Reasons, a.NoRoom)
	default:
		out.Best = true
	}
	switch {
	case a.Wall != "":
		out.Reasons = appendNew(out.Reasons, a.Wall)
	case a.NoRoomFit != "":
		out.Reasons = appendNew(out.Reasons, a.NoRoomFit)
	default:
		out.Fits = true
	}
	return out
}

func appendNew(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
