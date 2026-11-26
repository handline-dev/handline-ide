package cronspec

import (
	"testing"
	"time"
)

func TestDescribeSaysWhatTheExpressionNames(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		// every N minutes, and minutes of every hour
		{"* * * * *", "Every minute"},
		{"*/15 * * * *", "Every 15 minutes"},
		{"0-59/20 * * * *", "Every 20 minutes"},
		{"30 * * * *", "Every hour at :30"},
		{"5,35 * * * *", "Every hour at :05 and :35"},
		// daily at
		{"0 9 * * *", "Every day at 09:00"},
		{"5 0 * * *", "Every day at 00:05"},
		{"0 9,12,18 * * *", "Every day at 09:00, 12:00 and 18:00"},
		{"0 */5 * * *", "Every day at 00:00, 05:00, 10:00, 15:00 and 20:00"},
		// days of the week
		{"30 9 * * 1-5", "Every weekday at 09:30"},
		{"0 9 * * 1", "Every Monday at 09:00"},
		{"0 9 * * 7", "Every Sunday at 09:00"},
		{"0 9 * * 0,6", "Every Saturday and Sunday at 09:00"},
		{"0 9 * * 1,3,5", "Every Monday, Wednesday and Friday at 09:00"},
		{"0 9 * * 1-4", "Every day from Monday to Thursday at 09:00"},
		{"0 9 * * 5-7", "Every day from Friday to Sunday at 09:00"},
		{"0 9 1-31 * */2", "Every Tuesday, Thursday, Saturday and Sunday at 09:00"},
		// days of the month
		{"0 8 1 * *", "On the 1st of every month at 08:00"},
		{"0 8 1,15 * *", "On the 1st and 15th of every month at 08:00"},
		{"0 8 2,3,22,23 * *", "On the 2nd, 3rd, 22nd and 23rd of every month at 08:00"},
		{"0 8 1-7 * *", "From the 1st to the 7th of every month at 08:00"},
		{"0 8 */10 * *", "On the 1st, 11th, 21st and 31st of every month at 08:00"},
		{"0 0 29 2 *", "On the 29th of February at 00:00"},
		// both day fields written out: either one matches
		{"0 9 13 * 5", "On the 13th of every month, and also every Friday, at 09:00"},
		{"0 0 1 1,6 1", "On the 1st of January and June, and also every Monday in January and June, at 00:00"},
		{"0 9 1-31 * 1", "Every day at 09:00"},
		{"0 9 1 * 0-6", "Every day at 09:00"},
		// months
		{"0 9 * 1 *", "Every day in January at 09:00"},
		{"0 9 * 3-10 1", "Every Monday from March to October at 09:00"},
		{"0 9 1 3-10 *", "On the 1st of every month from March to October at 09:00"},
		{"*/15 * * 1 *", "Every day in January, every 15 minutes"},
		// steps and ranges of hours
		{"0 */2 * * *", "Every 2 hours from 00:00"},
		{"30 1-23/2 * * *", "Every 2 hours from 01:30"},
		{"0 9-17 * * 1-5", "Every weekday, every hour from 09:00 to 17:00"},
		{"*/15 9-17 * * 1-5", "Every weekday, every 15 minutes from 09:00 to 17:45"},
		{"*/15 * * * 1-5", "Every weekday, every 15 minutes"},
		{"* 9 * * *", "Every minute from 09:00 to 09:59"},
		{"0,30 9 * * *", "Every day at 09:00 and 09:30"},
		{"0,30 9,10 * * *", "Every 30 minutes from 09:00 to 10:30"},
		{"0,20 9,14 * * *", "Every day at 09:00, 09:20, 14:00 and 14:20"},
		{"  0   9 * * *  ", "Every day at 09:00"},
	} {
		if got, ok := Describe(c.expr); !ok || got != c.want {
			t.Errorf("%q: got %q (%v), want %q", c.expr, got, ok, c.want)
		}
	}
}

func TestDescribeGivesNoSentenceItCannotSayExactly(t *testing.T) {
	for _, expr := range []string{
		"*/7 * * * *",             // 0, 7 ... 56, then 4 minutes to the next hour: not every 7 minutes
		"5-59/15 9-17 * * *",      // a step that does not start on the hour, over hours
		"0-30 9 * * *",            // half an hour of minutes
		"*/15 9,13 * * *",         // a step over hours that are apart
		"0 9 */2 * 1",             // a stepped * and a weekday: both must match
		"0 9 1-7 * */2",           // the same, the other way round
		"0 9 1-20/2 * *",          // ten days of the month
		"0 9 * 1,3,5,7,9,11,12 *", // seven months apart
		"0 9 * *",                 // not an expression
		"61 9 * * *",
	} {
		if got, ok := Describe(expr); ok {
			t.Errorf("%q: no sentence says it exactly, got %q", expr, got)
		}
	}
}

// The sentences of the day rules are held against the times nextAfter gives, so a sentence cannot say one rule and the runner follow another.
func TestDescribeAgreesWithTheTimesTheRunnerFires(t *testing.T) {
	time.Local = time.UTC
	at := func(s string) int64 { v, _ := time.Parse("2006-01-02 15:04", s); return v.Unix() }
	for _, c := range []struct {
		expr, want, from string
		fires            []string
	}{
		// November 2026 starts on a Sunday; its Fridays are the 6th, 13th, 20th and 27th.
		{"0 9 13 * 5", "On the 13th of every month, and also every Friday, at 09:00", "2026-11-01 00:00",
			[]string{"2026-11-06 09:00", "2026-11-13 09:00", "2026-11-20 09:00", "2026-11-27 09:00", "2026-12-04 09:00", "2026-12-11 09:00",
				"2026-12-13 09:00"}},
		{"0 9 1-31 * 1", "Every day at 09:00", "2026-11-01 00:00", []string{"2026-11-01 09:00", "2026-11-02 09:00", "2026-11-03 09:00"}},
		{"0 9 * * 1", "Every Monday at 09:00", "2026-11-01 00:00", []string{"2026-11-02 09:00", "2026-11-09 09:00"}},
		{"0 9 1-31 * */2", "Every Tuesday, Thursday, Saturday and Sunday at 09:00", "2026-11-01 00:00",
			[]string{"2026-11-01 09:00", "2026-11-03 09:00", "2026-11-05 09:00", "2026-11-07 09:00", "2026-11-08 09:00"}},
		{"30 1-23/2 * * *", "Every 2 hours from 01:30", "2026-11-01 22:00", []string{"2026-11-01 23:30", "2026-11-02 01:30", "2026-11-02 03:30"}},
		{"*/15 9-17 * * 1-5", "Every weekday, every 15 minutes from 09:00 to 17:45", "2026-11-06 17:30",
			[]string{"2026-11-06 17:45", "2026-11-09 09:00", "2026-11-09 09:15"}},
		{"0 9-17 * * *", "Every hour from 09:00 to 17:00", "2026-11-01 16:30", []string{"2026-11-01 17:00", "2026-11-02 09:00"}},
	} {
		if got, ok := Describe(c.expr); !ok || got != c.want {
			t.Errorf("%q: got %q (%v), want %q", c.expr, got, ok, c.want)
		}
		ts := at(c.from)
		for _, want := range c.fires {
			next, ok := NextCronAfter(c.expr, ts)
			if !ok || next != at(want) {
				t.Errorf("%q after %s: fires %s, want %s", c.expr, time.Unix(ts, 0).UTC().Format("2006-01-02 15:04"),
					time.Unix(next, 0).UTC().Format("2006-01-02 15:04"), want)
				break
			}
			ts = next
		}
	}
}
