package verdict

import "testing"

func TestAgentStateFromStatusWaitingForAndTurn(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Input
		want State
	}{
		{"no session", Input{}, Closed},
		{"launching", Input{Launching: true}, Starting},
		{"launching with a file is its status", Input{Launching: true, Status: "idle"}, Idle},
		{"busy", Input{Status: "busy"}, Working},
		{"idle", Input{Status: "idle"}, Idle},
		{"idle with background work", Input{Status: "idle", Screen: ScreenBackground}, IdleWithBackground},
		{"busy ignores background work", Input{Status: "busy", Screen: ScreenBackground}, Working},
		{"permission prompt", Input{Status: "waiting", WaitingFor: "permission to run Bash"}, WaitingPermission},
		{"permission, any case", Input{Status: "waiting", WaitingFor: "Permission"}, WaitingPermission},
		{"question", Input{Status: "waiting", WaitingFor: "question"}, Dialog},
		{"usage limit choice", Input{Status: "waiting", WaitingFor: "usage limit"}, Dialog},
		{"waiting for nothing named", Input{Status: "waiting"}, Dialog},
		{"unreadable file", Input{Status: "unknown"}, Unknown},
		{"a status it does not know", Input{Status: "compacting"}, Unknown},
		{"shell left", Input{Screen: ScreenShell}, ShellLeft},
		{"turn working beats an idle file", Input{Status: "idle", Turn: TurnWorking}, Working},
		{"turn working beats no file", Input{Turn: TurnWorking}, Working},
		{"turn idle beats a busy file", Input{Status: "busy", Turn: TurnIdle}, Idle},
		{"turn idle beats a prompt", Input{Status: "waiting", WaitingFor: "permission", Turn: TurnIdle}, Idle},
	} {
		if got := AgentState(c.in); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFoldsOverEveryState(t *testing.T) {
	all := []State{Unknown, Starting, Working, IdleWithBackground, Idle, WaitingPermission, Dialog, ShellLeft, Closed}
	busy := map[State]bool{Working: true}
	blocked := map[State]bool{WaitingPermission: true, Dialog: true}
	safe := map[State]bool{Idle: true}
	for _, s := range all {
		if Busy(s) != busy[s] || Blocked(s) != blocked[s] || SafeToMove(s) != safe[s] {
			t.Errorf("state %d: busy %v blocked %v safe %v", s, Busy(s), Blocked(s), SafeToMove(s))
		}
	}
}
