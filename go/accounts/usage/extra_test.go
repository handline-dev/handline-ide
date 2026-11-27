package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/paths"
)

func f64(v float64) *float64 { return &v }

// A reading's extra usage reaches the status file, the account's status and its usage sample; a reading that names none leaves it
// unknown everywhere, never zero.
func TestExtraUsageGoesFromTheReadingToTheStatusAndTheSample(t *testing.T) {
	e, q, _ := burnEnv(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusFile(e.Home, "a")), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Reading{Account: "a", At: clock - 30, Windows: []ReadingWindow{{Minutes: 10080, Pct: pct(40)}},
		Extra: &Extra{On: true, Cap: f64(50), Spent: f64(14.5), Currency: "USD"}}
	for _, x := range []*Extra{r.Extra, nil} {
		r.Extra = x
		if !Record(e.Home, r) {
			t.Fatal("not written")
		}
		st := e.Read("a")
		if fmt.Sprint(st.Extra.Map()) != fmt.Sprint(x.Map()) {
			t.Fatalf("status: %v, want %v", st.Extra.Map(), x.Map())
		}
		if err := e.RecordSample(q, "a", st, 0, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := readSamples(q, 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	if got := ExtraOf(rows[0]["extra"]); got == nil || !got.On || *got.Cap != 50 || *got.Spent != 14.5 || got.Currency != "USD" {
		t.Fatalf("the first sample keeps it: %v", rows[0]["extra"])
	}
	if rows[1]["extra"] != nil {
		t.Fatalf("the second is unknown, not zero: %v", rows[1]["extra"])
	}
}

// Each day's extra usage is the rise of spent between an account's readings, dated by the later one; a drop is the month starting over.
func TestExtraDailyCountsRisesAndAMonthStartingOver(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local).Unix()
	row := func(acct string, h int, extra any) sampleRow {
		return sampleRow{"account": acct, "ts": json.Number(fmt.Sprint(day + int64(h)*3600)), "extra": extra}
	}
	usd := func(spent float64) string {
		return fmt.Sprintf(`{"on":true,"cap":null,"spent":%v,"currency":"USD"}`, spent)
	}
	rows := []sampleRow{
		row("a", -2, usd(10)),   // before the range: only what the next rise starts from
		row("a", 1, usd(12.5)),  // +2.50
		row("a", 2, nil),        // no extra_usage in that answer: unknown, skipped
		row("a", 3, usd(12.5)),  // no rise
		row("a", 26, usd(20)),   // next day +7.50
		row("a", 27, usd(1.25)), // the billing month began again: 1.25 from 0
		row("b", 1, `{"on":true,"cap":null,"spent":300,"currency":"jpy"}`),
		row("b", 4, `{"on":true,"cap":null,"spent":800,"currency":"jpy"}`),
		row("c", 5, `{"on":false,"cap":null,"spent":null,"currency":null}`),
	}
	got := fmt.Sprint(ExtraDaily(rows, day-1, day+48*3600))
	if want := "map[2026-11-20:map[JPY:500 USD:2.5] 2026-11-21:map[USD:8.75]]"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestExtraSpentAddsEachCurrencyApart(t *testing.T) {
	bs := []AccountBudget{
		{Extra: &Extra{On: true, Spent: f64(14.5), Currency: "USD"}},
		{Extra: &Extra{On: false, Spent: f64(2), Currency: "usd"}}, // turned off after spending: still spent this month
		{Extra: &Extra{On: true, Spent: f64(800), Currency: "JPY"}},
		{Extra: &Extra{On: false}},
		{},
	}
	if got := fmt.Sprint(ExtraSpent(bs)); got != "map[JPY:800 USD:16.5]" {
		t.Fatal(got)
	}
}

// Another machine's reading is the authority for extra usage as for its windows: one that names none makes it unknown.
func TestARunnersReadingCarriesItsExtraUsage(t *testing.T) {
	e, _, _ := burnEnv(t)
	if err := os.MkdirAll(filepath.Dir(paths.StatusFile(e.Home, "a")), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Reading{Account: "a", At: clock - 60, Windows: []ReadingWindow{{Minutes: 10080, Pct: pct(40)}},
		Extra: &Extra{On: true, Spent: f64(3), Currency: "USD"}}
	if !RecordNewer(e.Home, r) || e.Read("a").Extra == nil || *e.Read("a").Extra.Spent != 3 {
		t.Fatalf("the runner's extra usage was not kept: %v", e.Read("a").Extra)
	}
	r.At, r.Extra = clock, nil
	if RecordNewer(e.Home, r); e.Read("a").Extra != nil {
		t.Fatal("the runner's answer named none: unknown")
	}
}

// An extra figure read over a day ago is unknown in the budget: a stopped poll does not count last month's spend as this one's.
func TestAnOldExtraIsNotThisMonths(t *testing.T) {
	at := int64(clock - ExtraMaxAgeS - 1)
	st := &AccountStatus{CollectedAt: &at, AsOf: ptrF(float64(clock)), Extra: &Extra{On: true, Spent: f64(3)}}
	if BudgetOf(st, "Pro", 20, clock).Extra != nil {
		t.Fatal("a day-old extra was served")
	}
	at = clock - 60
	if BudgetOf(st, "Pro", 20, clock).Extra == nil {
		t.Fatal("a recent extra was dropped")
	}
}
