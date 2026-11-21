package states

// MovedWhy is a move's reason said from its trigger and the account it left only, for an event written before moves kept a reason (the
// lifecycle's moveReason adds the numbers).
func MovedWhy(trigger, from string) string {
	switch trigger {
	case "forced":
		return "You moved it."
	case "usage":
		return from + "'s 5-hour limit was reached."
	case "usage_7d":
		return from + "'s weekly limit was reached."
	case EarlyUsageTrigger:
		return from + " was close to its limit, so it moved before running out."
	case "login":
		return from + "'s login expired."
	case "ended":
		return from + "'s subscription ended."
	case "paused":
		return from + " was paused."
	}
	return "It moved off " + from + " when it started again."
}
