package words

import "testing"

func TestOrdinalAndAnd(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd",
		23: "23rd", 31: "31st", 101: "101st", 111: "111th"} {
		if got := Ordinal(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
	for want, items := range map[string][]string{"": nil, "a": {"a"}, "a and b": {"a", "b"}, "a, b and c": {"a", "b", "c"}} {
		if got := And(items); got != want {
			t.Errorf("%v: %q, want %q", items, got, want)
		}
	}
}
