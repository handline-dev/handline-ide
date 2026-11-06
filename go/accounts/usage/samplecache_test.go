package usage

import (
	"fmt"
	"reflect"
	"testing"
)

// The held rows are what a read gives, through new rows, a prune and a row written back in time.
func TestSampleCacheReadsWhatAReadGives(t *testing.T) {
	t.Parallel()
	q := openDB(t)
	add := func(acct string, ts int64) {
		if _, err := q.Exec(`INSERT INTO usage_samples(account, ts, exists_, windows) VALUES(?,?,1,'[{"minutes":10080,"used_percent":5}]')`, acct, ts); err != nil {
			t.Fatal(err)
		}
	}
	var c SampleCache
	same := func(since int64, why string) {
		t.Helper()
		got, err := c.Since(q, since)
		if err != nil {
			t.Fatal(err)
		}
		want, err := SamplesSince(q, since)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, Samples(want))) {
			t.Fatalf("%s: held %v, read %v", why, got, want)
		}
	}
	same(100, "empty")
	add("a", 100)
	add("b", 150)
	same(100, "first rows")
	add("a", 200)
	same(100, "a new row")
	same(150, "a later floor")
	same(50, "an earlier floor")
	if _, err := q.Exec("DELETE FROM usage_samples WHERE ts < 150"); err != nil {
		t.Fatal(err)
	}
	add("b", 250)
	same(50, "pruned and a new row")
	add("a", 180)
	same(50, "a row written back in time")
	if _, err := q.Exec("DELETE FROM usage_samples WHERE ts = 200"); err != nil {
		t.Fatal(err)
	}
	same(50, "a row deleted inside")
}

// After a prune, and past a row written before the floor, the held rows are kept and only the new rows are read, not all again.
func TestSampleCacheKeepsItsRowsThroughAPrune(t *testing.T) {
	t.Parallel()
	q := openDB(t)
	add := func(ts int64) {
		if _, err := q.Exec(`INSERT INTO usage_samples(account, ts, exists_, windows) VALUES('a',?,1,'[]')`, ts); err != nil {
			t.Fatal(err)
		}
	}
	for _, ts := range []int64{100, 200, 300} {
		add(ts)
	}
	var c SampleCache
	before, err := c.Since(q, 150)
	if err != nil || len(before) != 2 {
		t.Fatalf("%v %v", before, err)
	}
	if _, err := q.Exec("DELETE FROM usage_samples WHERE ts < 250"); err != nil {
		t.Fatal(err)
	}
	add(120) // before the floor: no row a read from 150 gives
	add(400)
	after, err := c.Since(q, 250)
	if err != nil || len(after) != 2 {
		t.Fatalf("%v %v", after, err)
	}
	if fmt.Sprintf("%p", after[0]) != fmt.Sprintf("%p", before[1]) {
		t.Fatal("the held row at 300 was read again")
	}
}
