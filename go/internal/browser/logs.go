package browser

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Console is what the page wrote to its console, its uncaught errors and the browser's own notes on it (a failed load), oldest first: the
// browser keeps them for the page as it is, so they are there for any command to read.
func (p *Page) Console() (string, error) {
	var lines []string
	prev := p.c.on
	p.c.on = func(m message) {
		if l := consoleLine(m); l != "" {
			lines = append(lines, l)
		}
		prev(m)
	}
	defer func() { p.c.on = prev }()
	for _, m := range []string{"Runtime.enable", "Log.enable"} { // each sends what it kept before it answers
		if _, err := p.c.call(m, nil); err != nil {
			return "", err
		}
	}
	return strings.Join(lines, "\n"), nil
}

type remote struct {
	Type        string          `json:"type"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
}

func (r remote) String() string {
	var s string
	if json.Unmarshal(r.Value, &s) == nil {
		return s
	}
	if len(r.Value) > 0 {
		return string(r.Value)
	}
	if r.Description != "" {
		return r.Description
	}
	return r.Type
}

type frame struct {
	URL  string `json:"url"`
	Line int    `json:"lineNumber"`
}

func at(url string, line int) string {
	if url == "" || strings.HasPrefix(url, "data:") {
		return ""
	}
	return fmt.Sprintf("  (%s:%d)", url, line+1)
}

func consoleLine(m message) string {
	switch m.Method {
	case "Runtime.consoleAPICalled":
		var e struct {
			Type  string   `json:"type"`
			Args  []remote `json:"args"`
			Stack struct {
				Frames []frame `json:"callFrames"`
			} `json:"stackTrace"`
		}
		_ = json.Unmarshal(m.Params, &e)
		parts := make([]string, len(e.Args))
		for i, a := range e.Args {
			parts[i] = a.String()
		}
		where := ""
		if len(e.Stack.Frames) > 0 {
			where = at(e.Stack.Frames[0].URL, e.Stack.Frames[0].Line)
		}
		return e.Type + "  " + strings.Join(parts, " ") + where
	case "Runtime.exceptionThrown":
		var e struct {
			Details struct {
				Text      string `json:"text"`
				URL       string `json:"url"`
				Line      int    `json:"lineNumber"`
				Exception *remote
			} `json:"exceptionDetails"`
		}
		_ = json.Unmarshal(m.Params, &e)
		text := e.Details.Text
		if x := e.Details.Exception; x != nil && x.Description != "" {
			text, _, _ = strings.Cut(x.Description, "\n")
		}
		return "error  " + text + at(e.Details.URL, e.Details.Line)
	case "Log.entryAdded":
		var e struct {
			Entry struct {
				Level, Text, URL string
				Line             int `json:"lineNumber"`
			} `json:"entry"`
		}
		_ = json.Unmarshal(m.Params, &e)
		return e.Entry.Level + "  " + e.Entry.Text + at(e.Entry.URL, e.Entry.Line)
	}
	return ""
}

// Network is what the page has loaded, the page itself first: each load's status (0 when it failed or the site does not say), kind, time
// and address.
func (p *Page) Network() (string, error) {
	var rows [][]any
	err := p.eval(`performance.getEntriesByType("navigation").concat(performance.getEntriesByType("resource")).map(e =>
		[e.responseStatus ?? 0, e.initiatorType || e.entryType, Math.round(e.duration), e.name])`, &rows)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, r := range rows {
		if len(r) == 4 {
			fmt.Fprintf(&out, "%v  %v  %vms  %v\n", r[0], r[1], r[2], r[3])
		}
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}
