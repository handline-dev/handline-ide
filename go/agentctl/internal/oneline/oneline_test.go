package oneline

import "testing"

// A line someone else wrote is drawn with nothing a terminal would obey: line breaks are spaces, escape sequences and other
// controls are gone, a tab is spaces, and what is printable stays.
func TestOnelineTakesOutEveryControl(t *testing.T) {
	for in, want := range map[string]string{
		"a\nb\x07c\u200bd":                "a bc d",
		"red \x1b[31mtext\x1b[0m":         "red text",
		"\x1b]0;title\x07after":           "after",
		"one\r\ntwo\u2028three\u0085four": "one two three four",
		"a\tb":                            "a   b",
		"del\x7f and c1\u009b":            "del and c1",
		"é ✓ plain":                       "é ✓ plain",
	} {
		if got := Oneline(in); got != want {
			t.Errorf("Oneline(%q) = %q, want %q", in, got, want)
		}
	}
	// A line break is a break, not a character blanked: each of these splits the text in two.
	for _, br := range []string{"\n", "\r\n", "\u2028", "\u0085", "\f"} {
		if got := splitLines("a" + br + "b"); len(got) != 2 {
			t.Errorf("splitLines(%q) = %q", "a"+br+"b", got)
		}
	}
}
