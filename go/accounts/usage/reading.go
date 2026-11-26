package usage

// Reading is what one poll of an account found: the shape U1's meter hands to usage.Record, and the only thing the status reader and
// the meter share. A nil Pct is a window the payload did not report.
type Reading struct {
	Account string
	At      int64 // unix seconds
	Windows []ReadingWindow
	// Extra is the account's extra usage as the answer named it; nil when it named none (unknown, never zero).
	Extra *Extra
}

// ReadingWindow is one window of a Reading.
type ReadingWindow struct {
	Minutes int
	Pct     *int
	Reset   int64 // 0: unknown
}
