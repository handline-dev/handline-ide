package joined

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/handline/handline/go/kit/atomicfile"
)

// Promotion is what a runner made the master (node.promote) leaves for the master it starts as: the master number it takes, the node
// that was master, whose sessions the board still has as its own, and whether that master was gone, so the new one tells it when it
// is back. Applied is that the first start wrote it into the board.
type Promotion struct {
	Number  int64  `json:"number"`
	From    string `json:"from,omitempty"`
	Tell    bool   `json:"tell,omitempty"`
	Applied bool   `json:"applied,omitempty"`
}

// PromotionFile is where a home keeps its Promotion until the master that was gone has been told.
func PromotionFile(home string) string { return filepath.Join(home, "promoted.json") }

// PromotionOf is home's Promotion; false when it keeps none.
func PromotionOf(home string) (Promotion, bool) {
	var p Promotion
	return p, readJSON(PromotionFile(home), &p)
}

// KeepPromotion keeps p in home.
func KeepPromotion(home string, p Promotion) error { return writeJSON(PromotionFile(home), p) }

// DropPromotion drops home's Promotion.
func DropPromotion(home string) error { return drop(PromotionFile(home)) }

// SetAside is a board a master set aside when a newer master outranked it: when, and the number that outranked it.
type SetAside struct {
	Since  int64 `json:"since"`
	Master int64 `json:"master"`
}

// SetAsideFile is where a home keeps its SetAside.
func SetAsideFile(home string) string { return filepath.Join(home, "set-aside.json") }

// SetAsideOf is home's SetAside; false when its master set none aside so.
func SetAsideOf(home string) (SetAside, bool) {
	var s SetAside
	return s, readJSON(SetAsideFile(home), &s)
}

// KeepSetAside keeps s in home.
func KeepSetAside(home string, s SetAside) error { return writeJSON(SetAsideFile(home), s) }

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, string(b))
}

func drop(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
