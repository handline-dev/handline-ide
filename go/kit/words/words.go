// Package words writes small things the way an English sentence says them: a place in a row, a list of names.
package words

import (
	"fmt"
	"strings"
)

// Ordinal is n as a place: 1st, 2nd, 3rd, 4th, 11th, 21st.
func Ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// And joins the items as a list: "a", "a and b", "a, b and c".
func And(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
