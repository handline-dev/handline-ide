package db

// Where a request stands is read from four columns: delivered_at (handed to the reader's session, or listed in a digest), typed_at
// (typed into its terminal), landed_at (in the session, or settled) and read_at (opened by hl read or on the page). Every SQL test that
// combines them is one of these, so its meaning is written once; a is the table's alias with its dot ("r."), or "".

// FounderOpen is a request to the founder still in their inbox: not yet dealt with, or a say not yet read.
func FounderOpen(a string) string {
	return "(" + a + "state = 'queued' OR (" + a + "kind = 'say' AND " + a + "state = 'landed' AND " + HasLanded(a) + " AND " + a + "read_at IS NULL))"
}

// NeverArrived is a request still waiting that was never seen to reach its reader: not landed, not read.
func NeverArrived(a string) string {
	return "(" + a + "state = 'queued' AND NOT " + HasLanded(a) + " AND " + a + "read_at IS NULL)"
}

// HasLanded is a request that reached its reader's session, or was settled there.
func HasLanded(a string) string { return a + "landed_at IS NOT NULL" }

// Digested is a notice a digest listed and nothing typed: the digest line carries it, so it is not shown again on its own.
func Digested(a string) string {
	return "(COALESCE(" + a + "notice, 0) >= 1 AND " + a + "delivered_at IS NOT NULL AND " + a + "typed_at IS NULL)"
}

// Held is a memo's notice kept for the reader's next digest: neither listed in one yet nor read.
func Held(a string) string {
	return "(" + a + "notice = 2 AND " + a + "delivered_at IS NULL AND " + a + "read_at IS NULL)"
}

// Unreached is a request handed to its reader's session that nothing says arrived: neither typed nor landed.
func Unreached(a string) string {
	return "(" + a + "delivered_at IS NOT NULL AND " + a + "typed_at IS NULL AND " + a + "landed_at IS NULL)"
}
