package adminwrites

import (
	"database/sql"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/scheduler/settings"
)

// ClusterNameMax is how long a cluster name may be, in characters.
const ClusterNameMax = 40

var (
	nameFirst = []string{"Amber", "Blue", "Bright", "Calm", "Cedar", "Clear", "Copper", "Coral", "Crimson", "Golden", "Green", "Hidden",
		"Iron", "Ivory", "Jade", "Lunar", "Maple", "Misty", "North", "Quiet", "Silver", "Slate", "Solar", "Swift", "Tidal", "Violet", "Wild"}
	nameSecond = []string{"Anchor", "Bay", "Brook", "Canyon", "Cove", "Delta", "Falls", "Field", "Forest", "Grove", "Harbor", "Haven",
		"Hill", "Island", "Lake", "Meadow", "Mesa", "Peak", "Pine", "Reef", "Ridge", "River", "Shore", "Summit", "Trail", "Valley", "Wharf"}
)

// RandomClusterName is two random words in Title Case ("Blue Harbor").
func RandomClusterName() string {
	return nameFirst[rand.IntN(len(nameFirst))] + " " + nameSecond[rand.IntN(len(nameSecond))]
}

// EnsureClusterName gives the board a random cluster name when it has none. The name is the board's (meta), so the copy a new master
// takes over carries it.
func EnsureClusterName(db *sql.DB) error {
	_, err := db.Exec(`INSERT INTO meta(name, value, updated) VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, updated = excluded.updated WHERE trim(COALESCE(meta.value, '')) = ''`,
		settings.ClusterNameKey, RandomClusterName(), time.Now().Unix())
	return err
}

func setClusterName(db *sql.DB, name string) error {
	_, err := db.Exec(`INSERT INTO meta(name, value, updated) VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, updated = excluded.updated`, settings.ClusterNameKey, name, time.Now().Unix())
	return err
}

// clusterName is v as a cluster name: trimmed, 1 to ClusterNameMax characters on one line.
func clusterName(v any) (string, bool) {
	s, ok := v.(string)
	s = strings.TrimSpace(s)
	return s, ok && s != "" && !textsafe.HasUnsafe(s) && utf8.RuneCountInString(s) <= ClusterNameMax
}
