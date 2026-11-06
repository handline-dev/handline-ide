package verdict

// ParkReason is why the daemon parked or queued an agent. It is stored beside the row, so nothing reads it out of the row's words.
type ParkReason string

const (
	ParkNone   ParkReason = ""
	ParkMoving ParkReason = "moving"  // the move machinery is waiting, not a usage wall
	ParkByHand ParkReason = "by_hand" // a person parked it
	ParkWall   ParkReason = "wall"    // the account hit its usage window
)

var parkReasons = map[ParkReason]bool{ParkMoving: true, ParkByHand: true, ParkWall: true}

// ParseParkReason is the reason a stored word names, ParkNone for none or a word it does not know.
func ParseParkReason(s string) ParkReason {
	if r := ParkReason(s); parkReasons[r] {
		return r
	}
	return ParkNone
}
