package dialogs

import "testing"

func TestOnlyARoutineDialogIsAnsweredAndAnUnknownOneIsAPersons(t *testing.T) {
	for id, want := range map[string]bool{"folder_trust": true, "tool_permission": false, "question": false,
		"usage_limit_choice": false, "exit_confirm": false, "startup_notice": false, "a dialog from the next release": false, "": false} {
		if Answered(id) != want {
			t.Errorf("%q answered: %v", id, !want)
		}
	}
	if e := Of("a dialog from the next release"); e.Class != Unknown || e.Kind != Dialog {
		t.Errorf("%+v", e)
	}
}

func TestTheUsageLimitDialogIsTheDaemonsAndStillNamesItsKind(t *testing.T) {
	if e := Of("usage_limit_choice"); e.Class != Daemon || e.Kind != Usage {
		t.Errorf("%+v", e)
	}
}
