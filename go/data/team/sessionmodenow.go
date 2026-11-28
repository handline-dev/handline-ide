package team

import "github.com/handline/handline/go/data/db"

// SessionModeNow is how the session in row runs now, in the SessionModes words: "" when none is open, "chat" while headless says its
// terminal runs under a Chat holder, else "terminal"; headless is asked only of an open one. agent.open_terminal and agent.get both ask
// it, so the offer and the action agree.
func SessionModeNow(row *db.AgentState, headless func(handle string) bool) string {
	if row == nil || row.Terminal.String == "" || (row.ClosedAt.Valid && row.ClosedAt.Int64 != 0) {
		return ""
	}
	if headless(row.Terminal.String) {
		return "chat"
	}
	return "terminal"
}
