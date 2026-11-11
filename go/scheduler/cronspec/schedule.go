package cronspec

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// The schedules spec (handline/team/schedules.py): read, validated and numbered the way schedules.list shows them.

// Schedule is one {cron, message}, as it is kept.
type Schedule struct{ Cron, Message string }

// ScheduleError is a `schedules` value that is wrong: what schedules.parse raises as a ValueError.
type ScheduleError struct{ Msg string }

func (e *ScheduleError) Error() string { return e.Msg }

// SID is what identifies a schedule: editing its cron or its message makes it a new one.
func SID(s Schedule) string {
	h := sha1.Sum([]byte(s.Cron + "\n" + s.Message))
	return hex.EncodeToString(h[:])[:10]
}

// ID is the integer the spec gives a schedule: ten hex digits of sha1 over the agent and the schedule's sid.
func ID(agent string, s Schedule) int64 {
	h := sha1.Sum([]byte(agent + "\n" + SID(s)))
	var n int64
	for _, b := range h[:5] {
		n = n<<8 | int64(b)
	}
	return n
}

// NextCronAfter is the first minute after ts the cron expression names, false when it names none within eight years or does not parse.
func NextCronAfter(expr string, ts int64) (int64, bool) {
	c, err := parseCron(expr)
	if err != nil {
		return 0, false
	}
	return c.nextAfter(ts)
}

// ParseSchedules validates the list and keeps each cron and message with their spaces squeezed (schedules.parse).
func ParseSchedules(items []Schedule) ([]Schedule, error) {
	out := make([]Schedule, 0, len(items))
	for i, it := range items {
		if _, err := parseCron(it.Cron); err != nil {
			return nil, &ScheduleError{err.Error()}
		}
		message := strings.Join(strings.Fields(it.Message), " ")
		if message == "" {
			return nil, &ScheduleError{fmt.Sprintf("schedule %d has no message", i+1)}
		}
		out = append(out, Schedule{strings.Join(strings.Fields(it.Cron), " "), message})
	}
	return out, nil
}

// pyText is str(v or "") for the values a schedule holds.
func pyText(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pyRepr is repr() of a string: single quotes unless it holds one and no double quote.
func pyRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	s = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`, q, `\`+q).Replace(s)
	return q + s + q
}

// badValue is a `schedules` value that is wrong, said as schedules.parse says it. Anything else that fails reading an agent is not one.
type badValue string

func (b badValue) Error() string { return string(b) }

// jsonError is json.loads's message where the text starts wrong, which is the usual mistake; Go's decoder words the rest.
func jsonError(text string, err error) string {
	t := strings.TrimLeft(text, " \t\r\n")
	literal := strings.HasPrefix(t, "true") || strings.HasPrefix(t, "false") || strings.HasPrefix(t, "null") || strings.HasPrefix(t, "NaN")
	if t == "" || (!literal && !strings.ContainsRune(`{["-0123456789`, rune(t[0]))) {
		return "Expecting value: line 1 column " + fmt.Sprint(len(text)-len(t)+1) + " (char " + fmt.Sprint(len(text)-len(t)) + ")"
	}
	return err.Error()
}

// load is an agent's `schedules` value validated as schedules.load does: a badValue says what is wrong with it, any other
// error is an agent row that cannot be read.
func load(home, agent string) ([]Schedule, error) {
	cfg, err := team.NewRoster(home).Read(agent)
	if err != nil {
		return nil, err
	}
	var items any
	switch v := cfg["schedules"].(type) {
	case nil:
		return nil, nil
	case string:
		if v == "" {
			return nil, nil
		}
		if err := json.Unmarshal([]byte(v), &items); err != nil {
			return nil, badValue("schedules is not JSON: " + jsonError(v, err))
		}
	default:
		items = v
	}
	list, ok := items.([]any)
	if !ok {
		return nil, badValue("schedules is a list of {cron, message}")
	}
	out := []Schedule{}
	for i, it := range list {
		var m map[string]any
		switch x := it.(type) {
		case map[string]any:
			m = x
		case *yamlcompat.OMap:
			m = x.M
		}
		_, hasC := m["cron"]
		_, hasM := m["message"]
		if len(m) != 2 || !hasC || !hasM {
			return nil, badValue(fmt.Sprintf("schedule %d is {cron, message} and nothing else", i+1))
		}
		cronText := pyText(m["cron"])
		if _, err := parseCron(cronText); err != nil {
			return nil, badValue(err.Error())
		}
		message := strings.Join(strings.Fields(pyText(m["message"])), " ")
		if message == "" {
			return nil, badValue(fmt.Sprintf("schedule %d has no message", i+1))
		}
		out = append(out, Schedule{strings.Join(strings.Fields(cronText), " "), message})
	}
	return out, nil
}

// Load is an agent's schedules; a *ScheduleError says what is wrong with its value, any other error is an agent row that cannot be read.
func Load(home, agent string) ([]Schedule, error) {
	items, err := load(home, agent)
	var bad badValue
	if errors.As(err, &bad) {
		return nil, &ScheduleError{string(bad)}
	}
	return items, err
}

// Of is Load, and none for an agent whose row or value cannot be read: the sweep's view.
func Of(home, agent string) []Schedule {
	items, err := Load(home, agent)
	if err != nil {
		return nil
	}
	return items
}
