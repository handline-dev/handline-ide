package verdict

import (
	"reflect"
	"testing"
)

func TestAdmitGivesEachTierAndEveryReasonAgainstTheAccount(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Account
		want Admission
	}{
		{"being removed refuses before anything else", Account{Removing: true, Excluded: true, ExcludeWhy: "busy", Assignable: true},
			Admission{Reasons: []string{"being removed"}}},
		{"set aside by the caller, with its why", Account{Excluded: true, ExcludeWhy: "the account it leaves", Assignable: true},
			Admission{Reasons: []string{"the account it leaves"}}},
		{"another vendor's", Account{NotFor: "an opencode account", Assignable: true, NoReading: true},
			Admission{Reasons: []string{"an opencode account"}}},
		{"a fresh reading with room is the best and fits", Account{Assignable: true},
			Admission{Best: true, Fits: true}},
		{"never run: the first fallback, and nothing known is against it", Account{AssignWhy: "no status.json", NoReading: true},
			Admission{NeverRun: true, Fits: true, Reasons: []string{"no reading yet - a session has never run on it"}}},
		{"a stale reading is not ranked, but it still fits", Account{AssignWhy: "the snapshot is stale (fail-closed)"},
			Admission{Fits: true, Reasons: []string{"the snapshot is stale (fail-closed)"}}},
		{"at the wall: neither tier, one reason when both say it",
			Account{AssignWhy: "5h usage at the wall (97%)", Wall: "5h usage at the wall (97%)"},
			Admission{Reasons: []string{"5h usage at the wall (97%)"}}},
		{"room on the ranked fill but not on the fresh one", Account{Assignable: true, NoRoomFit: "at its cap: 2 of 2 agents mid-turn"},
			Admission{Best: true, Reasons: []string{"at its cap: 2 of 2 agents mid-turn"}}},
		{"no room on either fill: both reasons", Account{Assignable: true, NoRoom: "short of its reset", NoRoomFit: "at its cap"},
			Admission{Reasons: []string{"short of its reset", "at its cap"}}},
	} {
		if got := Admit(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
		if got := Admit(c.in).OK(); got != (c.want.Best || c.want.NeverRun || c.want.Fits) {
			t.Errorf("%s: OK = %v", c.name, got)
		}
	}
	if !(Account{NotFor: "x"}).Outright() || !(Account{Excluded: true}).Outright() || (Account{AssignWhy: "stale"}).Outright() {
		t.Error("Outright is removing, set aside or another vendor's, nothing about a reading")
	}
}
