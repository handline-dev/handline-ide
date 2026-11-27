package yamlcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// cases are sources with what Parse reads them as, or refuses.
var cases = []struct {
	src, want string // want is JSON; empty when the source is refused
	refused   bool
}{
	{"a: 1\nb: two\nc: true\nd: no\ne: null\nf: ~\n", "{\"a\": 1, \"b\": \"two\", \"c\": true, \"d\": false, \"e\": null, \"f\": null}", false},
	{"a:\n  b: 1\n  c:\n    - x\n    - y\n", "{\"a\": {\"b\": 1, \"c\": [\"x\", \"y\"]}}", false},
	{"a:\n- x\n- y\nb: 2\n", "{\"a\": [\"x\", \"y\"], \"b\": 2}", false},
	{"items:\n  - name: a\n    n: 1\n  - name: b\n    n: 2\n", "{\"items\": [{\"name\": \"a\", \"n\": 1}, {\"name\": \"b\", \"n\": 2}]}", false},
	{"k: [a, 'b c', 3, 1.5]\nm: {x: 1, y: \"z\"}\n", "{\"k\": [\"a\", \"b c\", 3, 1.5], \"m\": {\"x\": 1, \"y\": \"z\"}}", false},
	{"url: http://example.com/a#frag\nr: go#1  # comment\n", "{\"url\": \"http://example.com/a#frag\", \"r\": \"go#1\"}", false},
	{"# only a comment\n", "{}", false},
	{"", "{}", false},
	{"---\na: 1\n", "{\"a\": 1}", false},
	{"a: 1\n---\nb: 2\n", "", true},
	{"a:\n\tb: 1\n", "", true},
	{"a: \"q\\\"r\\\\s\"\n", "{\"a\": \"q\\\"r\\\\s\"}", false},
	{"a: 'it''s'\n", "{\"a\": \"it's\"}", false},
	{"1: a\n", "", true},
	{"true: a\n", "", true},
	{"on: a\n", "", true},
	{"'true': a\n", "{\"true\": \"a\"}", false},
	{"a: [1, 2\n", "", true},
	{"a: 'x\n", "", true},
	{"a: 1\n b: 2\n", "", true},
	{"- a\n- b: 1\n  c: 2\n- [1, 2]\n- {k: v}\n", "[\"a\", {\"b\": 1, \"c\": 2}, [1, 2], {\"k\": \"v\"}]", false},
	{"a: 010\nb: 089\nc: 05593174\nd: 1e3\ne: 149628e2\n", "{\"a\": 8, \"b\": \"089\", \"c\": \"05593174\", \"d\": \"1e3\", \"e\": \"149628e2\"}", false},
	{"a: yes\nb: Off\nc: y\nd: yEs\ne: 'no'\n", "{\"a\": true, \"b\": false, \"c\": \"y\", \"d\": \"yEs\", \"e\": \"no\"}", false},
	{"---\n", "{}", false},
	{"null\n", "{}", false},
	{"&r {b: 1}\n", "", true},
	{"&k a: 1\n", "", true},
	{"!!str yes: 1\n", "", true},
	{"---\n# c\n", "{}", false},
	{"a: &x 1\nb: *x\n", "", true},
	{"a: &a [*a]\n", "", true},
	{"a: !!binary aGVsbG8=\n", "", true},
	{"a: !!int \"12\"\n", "", true},
	{"a: 0777777777777777777777777\n", "", true},
	{"a:b: c\n", "{\"a:b\": \"c\"}", false},
	{"  a: 1\n  b: 2\n", "{\"a\": 1, \"b\": 2}", false},
}

func TestParseReadsOrRefuses(t *testing.T) {
	for _, c := range cases {
		got, err := Parse(c.src)
		if c.refused {
			var ce *ConfigError
			if err == nil || !asConfigError(err, &ce) {
				t.Errorf("%q: want a ConfigError, got %v, %v", c.src, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		b, _ := json.Marshal(got)
		var gv, wv any
		json.Unmarshal(b, &gv)
		json.Unmarshal([]byte(c.want), &wv)
		if !reflect.DeepEqual(gv, wv) {
			t.Errorf("%q: got %s, want %s", c.src, b, c.want)
		}
	}
}

func asConfigError(err error, target **ConfigError) bool {
	ce, ok := err.(*ConfigError)
	*target = ce
	return ok
}

func TestIntegersAndFloatsAreKeptApart(t *testing.T) {
	got, err := Parse("a: 1\nb: 1.0\nc: 0x10\nd: -7\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"a": int64(1), "b": 1.0, "c": int64(16), "d": int64(-7)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%#v", got)
	}
	if _, err := Parse("a: 99999999999999999999\n"); err == nil {
		t.Fatal("a number past int64 was read")
	}
}

func TestDumpWritesWhatParseReadsBack(t *testing.T) {
	pairs := []Pair{{"name", "worker"}, {"n", 3}, {"flag", true}, {"f", 1e16}, {"none", nil}, {"word", "yes"}, {"colon", "a: b"}, {"num", "12"}, {"quote", `say "hi"`}, {"empty", ""},
		{"hosts", []any{"192.168.0.5", "mac.local", `a, "b"`}}, {"none_listed", []any{}}}
	text, err := Dump(pairs)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	want := map[string]any{"name": "worker", "n": int64(3), "flag": true, "f": 1e16, "none": nil, "word": "yes", "colon": "a: b", "num": "12", "quote": `say "hi"`, "empty": "",
		"hosts": []any{"192.168.0.5", "mac.local", `a, "b"`}, "none_listed": []any{}}
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("%#v\n%s", back, text)
	}
	if !strings.Contains(text, "f: 1.0e+16\n") {
		t.Fatalf("an exponent needs a decimal point: %s", text)
	}
	for _, bad := range []Pair{{"k", []any{1}}, {"k", []any{[]any{"a"}}}, {"k", []any{"a\nb"}}, {"k", "a\x1bb"}, {"k", "a\nb"}} {
		if _, err := Dump([]Pair{bad}); err == nil {
			t.Errorf("Dump wrote %#v", bad)
		}
	}
}

func TestIsPlainText(t *testing.T) {
	for name, want := range map[string]bool{"worker": true, "a.b-c": true, "on": false, "12": false, "a: b": false, "x,y": false} {
		if IsPlainText(name) != want {
			t.Errorf("%q: want %v", name, want)
		}
	}
}

func TestParsePairsKeepsTheFilesOrder(t *testing.T) {
	pairs, ok, err := ParsePairs("zeta: 1\nalpha:\n  - x\nmid: {b: 1, a: 2}\nalpha2: 3\n")
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	var keys []string
	for _, p := range pairs {
		keys = append(keys, p.Key)
	}
	if !reflect.DeepEqual(keys, []string{"zeta", "alpha", "mid", "alpha2"}) {
		t.Fatalf("%v", keys)
	}
	if _, ok, _ := ParsePairs("- a\n"); ok {
		t.Fatal("a list is not a map")
	}
	if _, _, err := ParsePairs("&r {b: 1}\n"); err == nil {
		t.Fatal("an anchored top level was taken")
	}
	if p, ok, err := ParsePairs("null\n"); err != nil || !ok || p != nil {
		t.Fatal("a null document is an empty map", p, ok, err)
	}
	if _, _, err := ParsePairs("1: x\n"); err == nil {
		t.Fatal("a key that is not text was taken")
	}
	dup, _, _ := ParsePairs("a: 1\nb: 2\na: 3\n")
	if len(dup) != 2 || dup[0] != (Pair{"a", int64(3)}) {
		t.Fatalf("a repeated key keeps its first place and its last value: %v", dup)
	}
}
