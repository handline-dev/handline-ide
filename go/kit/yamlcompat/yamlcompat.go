// Package yamlcompat reads and writes Handline's config files through yaml.v3. Values are nil, bool, int64, float64, string,
// []any and map[string]any; a map key must read as text. Dump writes only flat maps, in the order given.
package yamlcompat

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/handline/handline/go/kit/textsafe"
)

// ConfigError is what a file that cannot be read safely says.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func fail(format string, a ...any) error { return &ConfigError{fmt.Sprintf(format, a...)} }

var (
	// What Dump quotes: anything a YAML 1.1 reader might take for a boolean or null, in any case.
	yTrue  = map[string]bool{"true": true, "yes": true, "on": true, "y": true}
	yFalse = map[string]bool{"false": true, "no": true, "off": true, "n": true}
	// The YAML 1.1 boolean words a plain scalar reads as, in the three cases a 1.1 reader takes.
	boolWords = map[string]bool{"yes": true, "Yes": true, "YES": true, "on": true, "On": true, "ON": true,
		"no": false, "No": false, "NO": false, "off": false, "Off": false, "OFF": false}
	// Prefixes that are YAML syntax at the start of a plain value: anchors, aliases, tags, block scalars, merge keys.
	unsupported = []string{"&", "*", "|", ">", "!!", "<<"}
	leadingZero = regexp.MustCompile(`^[-+]?0[0-9]+$`)
	bigInt      = regexp.MustCompile(`^[-+]?[0-9]+$`)
	floatRe     = regexp.MustCompile(`^([-+]?[0-9]+\.[0-9]*|\.[0-9]+)([eE][-+][0-9]+)?$`)
)

// Pair is one key and value of a map, in order.
type Pair struct {
	Key string
	Val any
}

// root is the one document of text, nil when it is empty.
func root(text string) (*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fail("%s", err)
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, fail("several documents in one file")
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	n := doc.Content[0]
	if err := plainNode(n); err != nil {
		return nil, err
	}
	if n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" {
		return nil, nil // `---`, `~` or `null` alone
	}
	return n, nil
}

// Parse reads one YAML document into plain values; an empty one is an empty map.
func Parse(text string) (any, error) {
	n, err := root(text)
	if err != nil || n == nil {
		return map[string]any{}, err
	}
	return value(n)
}

// ParsePairs reads a document whose top level is a map and gives its keys in the order the file has them, so a rewrite keeps
// that order. ok is false when the top level is not a map.
func ParsePairs(text string) (pairs []Pair, ok bool, err error) {
	n, err := root(text)
	if err != nil {
		return nil, false, err
	}
	if n == nil {
		return nil, true, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, false, nil
	}
	err = eachKey(n, func(k string, v any) {
		for i := range pairs {
			if pairs[i].Key == k {
				pairs[i].Val = v
				return
			}
		}
		pairs = append(pairs, Pair{k, v})
	})
	return pairs, err == nil, err
}

func eachKey(n *yaml.Node, put func(string, any)) error {
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]
		if err := plainNode(kn); err != nil {
			return err
		}
		_, isBool := boolWords[kn.Value]
		if kn.Kind != yaml.ScalarNode || kn.ShortTag() != "!!str" || (isBool && kn.Style == 0) {
			return fail("line %d: key %s is not read as text; quote it if you mean the text", kn.Line, msgQuote(kn.Value))
		}
		v, err := value(vn)
		if err != nil {
			return err
		}
		put(kn.Value, v)
	}
	return nil
}

// plainNode refuses anchors, aliases and explicit tags, which only some readers agree on.
func plainNode(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return fail("line %d: anchors and aliases are not read; write the value out", n.Line)
	}
	if n.Style&yaml.TaggedStyle != 0 {
		return fail("line %d: tag %s is not read; write the value without it", n.Line, msgQuote(n.Tag))
	}
	return nil
}

func value(n *yaml.Node) (any, error) {
	if err := plainNode(n); err != nil {
		return nil, err
	}
	switch n.Kind {
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := value(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		m := map[string]any{}
		err := eachKey(n, func(k string, v any) { m[k] = v })
		return m, err
	}
	if b, ok := boolWords[n.Value]; ok && n.Style == 0 {
		return b, nil
	}
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		err := n.Decode(&b)
		return b, wrap(n, err)
	case "!!int":
		if leadingZero.MatchString(n.Value) {
			return octalOrText(n)
		}
		var i int64
		if err := n.Decode(&i); err != nil {
			return nil, fail("line %d: %s is too large for this reader; quote it if you mean the string", n.Line, msgQuote(n.Value))
		}
		return i, nil
	case "!!float":
		if leadingZero.MatchString(n.Value) {
			return octalOrText(n)
		}
		if bigInt.MatchString(n.Value) {
			return nil, fail("line %d: %s is too large for this reader; quote it if you mean the string", n.Line, msgQuote(n.Value))
		}
		if !IsFloat(n.Value) {
			return n.Value, nil // 1e3, .inf: a float needs a decimal point, as Dump writes one
		}
		var f float64
		err := n.Decode(&f)
		return f, wrap(n, err)
	}
	return n.Value, nil
}

// octalOrText reads a number with a leading zero as YAML 1.1 does: 010 is octal 8; 089 is no number, so it stays text.
func octalOrText(n *yaml.Node) (any, error) {
	i, err := strconv.ParseInt(n.Value, 8, 64)
	switch {
	case err == nil:
		return i, nil
	case errors.Is(err, strconv.ErrRange):
		return nil, fail("line %d: %s is too large for this reader; quote it if you mean the string", n.Line, msgQuote(n.Value))
	}
	return n.Value, nil
}

func wrap(n *yaml.Node, err error) error {
	if err == nil {
		return nil
	}
	return fail("line %d: %s", n.Line, err)
}

// msgQuote quotes a string for a message: single quotes (\' and \\ escaped), or double quotes when it holds a ' and no ".
func msgQuote(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	if q == "'" {
		s = strings.ReplaceAll(s, "'", `\'`)
	}
	return q + s + q
}

// IsPlainText is whether name is read back as itself wherever Handline writes one unquoted: as a block key, as a value, as a list
// item and as a flow key. A word a YAML 1.1 reader takes for a boolean or null is not.
func IsPlainText(name string) bool {
	if low := strings.ToLower(name); yTrue[low] || yFalse[low] || low == "null" || low == "~" {
		return false
	}
	shapes := []struct {
		src  string
		want any
	}{
		{name + ": 1\n", map[string]any{name: int64(1)}},
		{"k: " + name + "\n", map[string]any{"k": name}},
		{"k: [" + name + "]\n", map[string]any{"k": []any{name}}},
		{"k: { " + name + ": 1 }\n", map[string]any{"k": map[string]any{name: int64(1)}}},
	}
	for _, s := range shapes {
		got, err := Parse(s.src)
		if err != nil || !reflect.DeepEqual(got, s.want) {
			return false
		}
	}
	return true
}

// floatRepr is the shortest decimal that reads back as f, with ".0" when whole; an exponent below -4 or from 16 is written as
// mantissa, "e", sign and at least two digits (1e+16).
func floatRepr(f float64) string {
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exps, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(exps)
	if exp >= -4 && exp < 16 {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.ContainsAny(s, ".") {
			s += ".0"
		}
		return s
	}
	sign := "+"
	if exp < 0 {
		sign, exp = "-", -exp
	}
	return fmt.Sprintf("%se%s%02d", mant, sign, exp)
}

// floatText is a float as a YAML 1.1 reader also reads back as a float: an exponent only after a decimal point.
func floatText(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fail("dump does not write %v", f)
	}
	text := floatRepr(f)
	mant, exp, has := strings.Cut(text, "e")
	if strings.Contains(mant, ".") || !has {
		return text, nil
	}
	return mant + ".0e" + exp, nil
}

func quoted(text string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), `"`, `\"`) + `"`
}

func anyUnsafe(s string) bool {
	for _, r := range s {
		if textsafe.HasUnsafe(string(r)) {
			return true
		}
	}
	return false
}

// textList is a list of text with each item quoted for a flow list; ok is false for anything else.
func textList(v any) (quotedItems []string, ok bool) {
	l, isList := v.([]any)
	if !isList {
		return nil, false
	}
	for _, x := range l {
		s, isText := x.(string)
		if !isText {
			return nil, false
		}
		quotedItems = append(quotedItems, quoted(s))
	}
	return quotedItems, true
}

func normal(v any) any {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	}
	return v
}

// Dump writes a flat map back to YAML: scalars and one-line flow lists of text, nothing deeper. A list stays on one line
// (`key: [a, b]`) because the desktop app reads network_addresses from config.yaml by that shape. Every line is read back
// through Parse before it is kept, so a value the read path would refuse or change is an error here, at the write.
func Dump(data []Pair) (string, error) {
	var lines []string
	for _, p := range data {
		value := normal(p.Val)
		list, isList := textList(value)
		switch value.(type) {
		case map[string]any, []any:
			if !isList {
				return "", fail("dump does not write nested values (key %s)", msgQuote(p.Key))
			}
		}
		var candidates []string
		switch v := value.(type) {
		case []any:
			for _, s := range list {
				if anyUnsafe(s) {
					return "", fail("dump does not write control characters (key %s)", msgQuote(p.Key))
				}
			}
			candidates = []string{"[" + strings.Join(list, ", ") + "]"}
		case nil:
			candidates = []string{"null"}
		case bool:
			candidates = []string{strconv.FormatBool(v)}
		case int64:
			candidates = []string{strconv.FormatInt(v, 10)}
		case float64:
			t, err := floatText(v)
			if err != nil {
				return "", err
			}
			candidates = []string{t}
		default:
			s := fmt.Sprint(v)
			value = s
			if anyUnsafe(s) {
				return "", fail("dump does not write control characters (key %s)", msgQuote(p.Key))
			}
			low := strings.ToLower(s)
			needs := strings.TrimFunc(s, unicode.IsSpace) != s || s == "" || yTrue[low] || yFalse[low] || low == "null" ||
				low == "~" || strings.ContainsRune("-?!%@`=", rune(s[0])) || hasAnyPrefix(s, unsupported) ||
				strings.ContainsAny(s, `:#[]{},&*|>'"`) || IsFloat(s)
			if needs {
				candidates = []string{quoted(s)}
			} else {
				candidates = []string{s, quoted(s)}
			}
		}
		wrote := false
		for _, rendered := range candidates {
			line := p.Key + ": " + rendered
			back, err := Parse(line + "\n")
			if err != nil {
				continue
			}
			if m, ok := back.(map[string]any); ok && reflect.DeepEqual(normal(m[p.Key]), value) {
				lines = append(lines, line)
				wrote = true
				break
			}
		}
		if !wrote {
			return "", fail("dump cannot write %s: the parser does not read %s back as %v", msgQuote(p.Key), msgQuote(candidates[len(candidates)-1]), value)
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// IsFloat is whether a plain scalar reads as a float: it needs a decimal point, so 149628e2 is a string.
func IsFloat(s string) bool { return floatRe.MatchString(s) }

// FloatRepr is f as floatRepr writes it.
func FloatRepr(f float64) string { return floatRepr(f) }
