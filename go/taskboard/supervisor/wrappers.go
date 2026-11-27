package supervisor

import "github.com/handline/handline/go/data/db"

// Exported entry points over the readers that raise a failed statement internally (see Catch): each returns it as an error.

// RaiseDecision is raiseDecision with a failed statement returned as the error.
func (e *Env) RaiseDecision(q db.Q, agent, text, reason, files string, chainID string, depth int64, parentID int64, extra ...db.KV) (r0 int64, err error) {
	defer Catch(&err)
	r0 = e.raiseDecision(q, agent, text, reason, files, chainID, depth, parentID, extra...)
	return r0, nil
}

// Report is report with a failed statement returned as the error.
func (e *Env) Report(q db.Q, agent, text, reason string) (r0 int64, err error) {
	defer Catch(&err)
	r0 = e.report(q, agent, text, reason)
	return r0, nil
}

// IdleSince is idleSince with a failed statement returned as the error.
func (e *Env) IdleSince(q db.Q, agent string) (r0 int64, r1 bool, err error) {
	defer Catch(&err)
	r0, r1 = e.idleSince(q, agent)
	return r0, r1, nil
}

// HasBackgroundWork is hasBackgroundWork with a failed statement returned as the error.
func (e *Env) HasBackgroundWork(q db.Q, agent string) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.hasBackgroundWork(q, agent)
	return r0, nil
}

// BgOwesNothing is bgOwesNothing with a failed statement returned as the error.
func (e *Env) BgOwesNothing(q db.Q, agent string) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.bgOwesNothing(q, agent)
	return r0, nil
}

// BgPromptedAt is bgPromptedAt with a failed statement returned as the error.
func (e *Env) BgPromptedAt(q db.Q, agent string) (r0 int64, err error) {
	defer Catch(&err)
	r0 = e.bgPromptedAt(q, agent)
	return r0, nil
}

// BgSince is bgSince with a failed statement returned as the error.
func (e *Env) BgSince(q db.Q, agent string) (r0 int64, r1 bool, err error) {
	defer Catch(&err)
	r0, r1 = e.bgSince(q, agent)
	return r0, r1, nil
}

// BgSilentS is bgSilentS with a failed statement returned as the error.
func (e *Env) BgSilentS(q db.Q, agent string, now int64) (r0 int64, r1 bool, err error) {
	defer Catch(&err)
	r0, r1 = e.bgSilentS(q, agent, now)
	return r0, r1, nil
}

// BgStale is bgStale with a failed statement returned as the error.
func (e *Env) BgStale(q db.Q, agent string, now int64) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.bgStale(q, agent, now)
	return r0, nil
}

// BgMayStop is bgMayStop with a failed statement returned as the error.
func (e *Env) BgMayStop(q db.Q, agent string, now int64) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.bgMayStop(q, agent, now)
	return r0, nil
}

// BgHolds is bgHolds with a failed statement returned as the error.
func (e *Env) BgHolds(q db.Q, agent string, now int64) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.bgHolds(q, agent, now)
	return r0, nil
}

// ForgottenResolve is forgottenResolve with a failed statement returned as the error.
func (e *Env) ForgottenResolve(q db.Q, agent string, row *db.AgentState, now int64) (r0 *Forgotten, err error) {
	defer Catch(&err)
	r0 = e.forgottenResolve(q, agent, row, now)
	return r0, nil
}

// NudgeExhausted is nudgeExhausted with a failed statement returned as the error.
func (e *Env) NudgeExhausted(q db.Q, agent string, headID int64) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.nudgeExhausted(q, agent, headID)
	return r0, nil
}

// HeadTaskBlocked is headTaskBlocked with a failed statement returned as the error.
func (e *Env) HeadTaskBlocked(q db.Q, agent string, head *db.Request) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.headTaskBlocked(q, agent, head)
	return r0, nil
}

// EffectiveHead is effectiveHead with a failed statement returned as the error.
func (e *Env) EffectiveHead(q db.Q, agent string) (r0 *db.Request, err error) {
	defer Catch(&err)
	r0 = e.effectiveHead(q, agent)
	return r0, nil
}

// EffectiveTaskHead is effectiveTaskHead with a failed statement returned as the error.
func (e *Env) EffectiveTaskHead(q db.Q, agent string) (r0 *db.Request, err error) {
	defer Catch(&err)
	r0 = e.effectiveTaskHead(q, agent)
	return r0, nil
}

// SessionPresent is sessionPresent with a failed statement returned as the error.
func (e *Env) SessionPresent(agent string) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.sessionPresent(agent)
	return r0, nil
}

// WallReason is wallReason with a failed statement returned as the error.
func (e *Env) WallReason(q db.Q, agent string, row *db.AgentState) (r0 string, err error) {
	defer Catch(&err)
	r0 = e.wallReason(q, agent, row)
	return r0, nil
}

// SessionChanged is sessionChanged with a failed statement returned as the error.
func (e *Env) SessionChanged(q db.Q, agent string) (r0 bool, err error) {
	defer Catch(&err)
	r0 = e.sessionChanged(q, agent)
	return r0, nil
}

// PastDialog is pastDialog (delivery._past_dialog) with a failed statement returned as the error.
func (e *Env) PastDialog(q db.Q, agent string) (why string, err error) {
	defer Catch(&err)
	return e.pastDialog(q, agent, false), nil
}
