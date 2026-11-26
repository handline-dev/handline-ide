package cronspec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/handline/handline/go/kit/words"
)

// A cron expression said as a sentence, for a person reading a list of schedules. Every time in it is the daemon machine's local time,
// as the expression's own fields are. It says exactly what nextAfter does or nothing: an expression with no exact short sentence gets
// none, and its reader is shown the expression itself.

// listMax is the most times, days or months a sentence spells out one by one.
const listMax = 6

var (
	weekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	monthNames   = [13]string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October",
		"November", "December"}
)

// Describe is the expression as a sentence ("Every weekday at 09:00"), false when it does not parse or has no exact short sentence.
func Describe(expr string) (string, bool) {
	c, err := parseCron(expr)
	if err != nil {
		return "", false
	}
	clock, interval, ok := clockPhrase(sorted(c.fields[0]), sorted(c.fields[1]))
	if !ok {
		return "", false
	}
	days, kind, ok := c.dayPhrase()
	if !ok {
		return "", false
	}
	switch {
	case days == everyDay && interval: // "Every 15 minutes" says the days too
		return strings.ToUpper(clock[:1]) + clock[1:], true
	case kind == daysEither || interval:
		return days + ", " + clock, true
	}
	return days + " " + clock, true
}

// everyDay is the days of an expression that restricts none.
const everyDay = "Every day"

func sorted(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// stepOf is the step of values that go round a cycle of period evenly (0, 15, 30, 45 of 60), so the gap from the last to the first of
// the next cycle is the step too; false for one value, for every value, and for a step the cycle does not divide by.
func stepOf(vals []int, period int) (int, bool) {
	if len(vals) < 2 || len(vals) == period {
		return 0, false
	}
	step := vals[1] - vals[0]
	if period%step != 0 || len(vals) != period/step || vals[0] >= step {
		return 0, false
	}
	for i := 1; i < len(vals); i++ {
		if vals[i]-vals[i-1] != step {
			return 0, false
		}
	}
	return step, true
}

// unbroken is whether the values are each one more than the one before.
func unbroken(vals []int) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1]+1 {
			return false
		}
	}
	return true
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// clockPhrase is the times of day the minutes and hours name. interval says it reads as a repeat ("every 15 minutes") and not as times
// ("at 09:00"), which decides how the days go with it.
func clockPhrase(mins, hours []int) (text string, interval, ok bool) {
	at := func(h, m int) string { return fmt.Sprintf("%02d:%02d", h, m) }
	times := func() string {
		var all []string
		for _, h := range hours {
			for _, m := range mins {
				all = append(all, at(h, m))
			}
		}
		return "at " + words.And(all)
	}
	count := len(mins) * len(hours)
	minuteStep, minuteStepped := stepOf(mins, 60)
	minuteStepped = minuteStepped && mins[0] == 0
	if len(hours) == 24 {
		switch {
		case len(mins) == 60:
			return "every minute", true, true
		case len(mins) == 1:
			return fmt.Sprintf("every hour at :%02d", mins[0]), true, true
		case minuteStepped:
			return "every " + plural(minuteStep, "minute"), true, true
		case len(mins) <= listMax:
			past := make([]string, len(mins))
			for i, m := range mins {
				past[i] = fmt.Sprintf(":%02d", m)
			}
			return "every hour at " + words.And(past), true, true
		}
		return "", false, false
	}
	first, last := at(hours[0], mins[0]), at(hours[len(hours)-1], mins[len(mins)-1])
	hourStep, hourStepped := stepOf(hours, 24)
	switch {
	case count <= 3:
		return times(), false, true
	case len(mins) == 1 && hourStepped:
		return fmt.Sprintf("every %s from %s", plural(hourStep, "hour"), first), true, true
	case len(mins) == 1 && unbroken(hours):
		return fmt.Sprintf("every hour from %s to %s", first, last), true, true
	case len(mins) == 60 && unbroken(hours):
		return fmt.Sprintf("every minute from %s to %s", first, last), true, true
	case minuteStepped && unbroken(hours):
		return fmt.Sprintf("every %s from %s to %s", plural(minuteStep, "minute"), first, last), true, true
	case count <= listMax:
		return times(), false, true
	}
	return "", false, false
}

// How the two day fields restrict the days.
const (
	daysEvery   = iota // every day of the months named
	daysOfWeek         // only days of the week
	daysOfMonth        // only days of the month
	daysEither         // a day of the month or a day of the week: either one matches
)

// dayPhrase is the days the expression names, by dayOK's rule: a day field that starts with * leaves the choice to the other one, and
// with both written out a day matches when either does.
func (c *cronExpr) dayPhrase() (string, int, bool) {
	dom, mon, dow := sorted(c.fields[2]), sorted(c.fields[3]), sorted(c.fields[4])
	domAll, dowAll := len(dom) == 31, len(dow) == 7
	kind := daysEither
	switch {
	case c.domAny || c.dowAny:
		switch {
		case domAll && dowAll:
			kind = daysEvery
		case domAll:
			kind = daysOfWeek
		case dowAll:
			kind = daysOfMonth
		default: // a stepped * beside a restricted field: both must match
			return "", 0, false
		}
	case domAll || dowAll: // either matches, and one of them matches every day
		kind = daysEvery
	}
	in, of, ok := monthPhrases(mon)
	if !ok {
		return "", 0, false
	}
	week, month := weekPhrase(dow), ""
	if kind == daysOfMonth || kind == daysEither {
		if month, ok = monthDayPhrase(dom); !ok {
			return "", 0, false
		}
		month += " of " + of
	}
	switch kind {
	case daysEvery:
		return everyDay + in, kind, true
	case daysOfWeek:
		return strings.ToUpper(week[:1]) + week[1:] + in, kind, true
	case daysOfMonth:
		return month, kind, true
	}
	return month + ", and also " + week + in, kind, true
}

// monthPhrases is the months as they follow "every day" (" in January", "" for all) and as they follow a day of the month ("January",
// "every month" for all).
func monthPhrases(mon []int) (in, of string, ok bool) {
	names := make([]string, len(mon))
	for i, m := range mon {
		names[i] = monthNames[m]
	}
	switch {
	case len(mon) == 12:
		return "", "every month", true
	case len(mon) >= 3 && unbroken(mon):
		span := "from " + names[0] + " to " + names[len(names)-1]
		return " " + span, "every month " + span, true
	case len(mon) <= listMax:
		return " in " + words.And(names), words.And(names), true
	}
	return "", "", false
}

// weekPhrase is the days of the week, Monday first: "every weekday", "every Monday and Thursday", "every day from Monday to Thursday".
func weekPhrase(dow []int) string {
	var days []int // Monday 1 to Sunday 7
	for _, d := range dow {
		if d != 0 {
			days = append(days, d)
		}
	}
	if len(dow) > 0 && dow[0] == 0 {
		days = append(days, 7)
	}
	name := func(d int) string { return weekdayNames[d%7] }
	if len(days) == 5 && days[0] == 1 && days[4] == 5 {
		return "every weekday"
	}
	if len(days) >= 3 && unbroken(days) {
		return "every day from " + name(days[0]) + " to " + name(days[len(days)-1])
	}
	names := make([]string, len(days))
	for i, d := range days {
		names[i] = name(d)
	}
	return "every " + words.And(names)
}

// monthDayPhrase is the days of the month: "On the 1st and 15th", "From the 1st to the 7th".
func monthDayPhrase(dom []int) (string, bool) {
	if len(dom) >= 3 && unbroken(dom) {
		return "From the " + words.Ordinal(dom[0]) + " to the " + words.Ordinal(dom[len(dom)-1]), true
	}
	if len(dom) > listMax {
		return "", false
	}
	days := make([]string, len(dom))
	for i, d := range dom {
		days[i] = words.Ordinal(d)
	}
	return "On the " + words.And(days), true
}
