package filerw

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// section is one part of a ChangeSet: keep n units, or replace n units with ins.
type section struct {
	n       int
	replace bool
	ins     []uint16
}

// changeSet is CodeMirror's ChangeSet, read from its toJSON form: lengths in UTF-16 code units.
type changeSet []section

var errChangeSet = errors.New("not a ChangeSet")

// parseChangeSet reads ChangeSet.toJSON(): an integer keeps that many units, [n, ...lines] replaces n units with the lines joined by \n.
func parseChangeSet(v any) (changeSet, error) {
	parts, ok := v.([]any)
	if !ok {
		return nil, errChangeSet
	}
	cs := make(changeSet, 0, len(parts))
	for _, p := range parts {
		switch p := p.(type) {
		case []any:
			if len(p) == 0 {
				return nil, errChangeSet
			}
			n, ok := count(p[0])
			if !ok {
				return nil, errChangeSet
			}
			lines := make([]string, 0, len(p)-1)
			for _, l := range p[1:] {
				s, ok := l.(string)
				if !ok || !utf8.ValidString(s) || strings.ContainsAny(s, "\x00\r\n") {
					return nil, errChangeSet
				}
				lines = append(lines, s)
			}
			var ins []uint16
			if len(lines) > 0 {
				ins = utf16.Encode([]rune(strings.Join(lines, "\n")))
			}
			cs = append(cs, section{n: n, replace: true, ins: ins})
		default:
			n, ok := count(p)
			if !ok {
				return nil, errChangeSet
			}
			cs = append(cs, section{n: n})
		}
	}
	return cs, nil
}

// count is v as a length: a whole number from 0, as a call's params hold it.
func count(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return int(i), err == nil && i >= 0
}

// apply is the text after cs, or an error when cs's lengths do not add up to the text's.
func (cs changeSet) apply(text []uint16) ([]uint16, error) {
	out := make([]uint16, 0, len(text))
	at := 0
	for _, s := range cs {
		if s.n > len(text)-at { // not at+s.n, which a length near the int maximum wraps
			return nil, errChangeSet
		}
		if s.replace {
			out = append(out, s.ins...)
		} else {
			out = append(out, text[at:at+s.n]...)
		}
		at += s.n
	}
	if at != len(text) {
		return nil, errChangeSet
	}
	return out, nil
}
