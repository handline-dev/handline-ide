package rulesfold

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
)

// check validates the model's rule set against the rules kept before and the items it read, and gives the rules to write and the kept
// ones it drops. An id or merge that names no kept rule is left out: a new rule. Every kept rule must come back, by its id or merged
// into another, unless the fold may drop it (one it wrote, said once). No count goes down, and none goes up by more than the items read
// that say it. A quote names an item read or the quote it had. An item's kind of work is its quote's area. A rule that comes back as it
// was kept is not checked again, so a person's long rule stands as they wrote it. When an area is past its cap, the fold's own rules
// go, the least said first and new ones before kept ones, until it fits; a person's never go, and a set that cannot fit is refused.
func check(person string, before []kept, out []outRule, items []Item) (rules, dropped []Rule, err error) {
	old := map[int64]kept{}
	for _, k := range before {
		old[k.ID] = k
	}
	byRef := map[string]Item{}
	for _, it := range items {
		byRef[it.Ref] = it
	}
	claimed := map[int64]bool{}
	keep := map[int]bool{} // index in rules: may not be dropped to fit
	for _, o := range out {
		r := o.Rule
		if _, ok := old[r.ID]; !ok {
			r.ID = 0 // an id the model made up is a new rule: nothing kept can be lost by it
		}
		var ids []int64
		if r.ID != 0 {
			ids = append(ids, r.ID)
		}
		for _, m := range o.Merges {
			if _, ok := old[m]; ok {
				ids = append(ids, m)
			}
		}
		prior, quotes, protected := 0, map[string]bool{}, false
		var was *kept
		for _, id := range ids {
			k := old[id]
			if claimed[id] {
				return nil, nil, fmt.Errorf("rule %d comes back twice", id)
			}
			claimed[id] = true
			prior += k.Count
			quotes[k.QuoteRef] = true
			protected = protected || k.keep
			if r.Task == 0 && k.QuoteRef == r.QuoteRef {
				r.Task = k.Task
			}
			if len(ids) == 1 {
				was = &k
			}
		}
		read := 0
		for _, ref := range o.Refs {
			if _, ok := byRef[ref]; ok {
				read++
			}
		}
		if r.Count < max(prior, 1) {
			return nil, nil, fmt.Errorf("rule %q is said %d times, fewer than the %d it had", r.Text, r.Count, prior)
		}
		r.Count = min(r.Count, max(prior+read, 1)) // a count rises only by the items read that say it
		if was != nil && same(was.Rule, r) {
			r.Task, r.stored = was.Task, was.Text
		} else {
			// a person's own rule keeps its words however long; only the model's are held to the limits
			if was == nil || !was.keep || r.Text != was.Rule.Text {
				if err := checkText(r); err != nil {
					return nil, nil, err
				}
			}
			if it, ok := byRef[r.QuoteRef]; ok {
				r.Task = it.task
				if it.Kind != "" {
					r.Area = it.Kind
				}
			} else if !quotes[r.QuoteRef] {
				return nil, nil, fmt.Errorf("rule %q quotes %q, which is neither an item read nor its own quote", r.Text, r.QuoteRef)
			}
			if r.QuoteRef == "" {
				r.Quote = ""
			}
		}
		r.from = ids
		if protected {
			keep[len(rules)] = true
		}
		rules = append(rules, r)
	}
	for _, k := range before {
		if claimed[k.ID] {
			continue
		}
		if k.keep || k.Count > 1 {
			return nil, nil, fmt.Errorf("kept rule %d (%q) is gone", k.ID, k.Text)
		}
		r := k.Rule
		r.from = []int64{k.ID}
		dropped = append(dropped, r)
	}
	return fit(person, rules, keep, dropped)
}

func checkText(r Rule) error {
	switch {
	case r.Area != db.AreaScreen && r.Area != db.AreaWork:
		return fmt.Errorf("rule %q has area %q", r.Text, r.Area)
	case r.Text == "" || utf8.RuneCountInString(r.Text) > maxText || strings.ContainsAny(r.Text, "\n\r"):
		return fmt.Errorf("rule text %q is empty, over %d characters or more than a line", r.Text, maxText)
	case r.QuoteRef != "" && (r.Quote == "" || utf8.RuneCountInString(r.Quote) > maxQuote || strings.ContainsAny(r.Quote, "\n\r")):
		return fmt.Errorf("rule %q has a quote that is empty, over %d characters or more than a line", r.Text, maxQuote)
	}
	return nil
}

// fit drops the fold's own rules, the least said first and new ones before kept ones, until each area fits its cap. A dropped rule
// that replaces kept ones takes them with it.
func fit(person string, rules []Rule, keep map[int]bool, dropped []Rule) ([]Rule, []Rule, error) {
	lessons := func(rs []Rule) []db.Lesson {
		out := make([]db.Lesson, 0, len(rs))
		for _, r := range rs {
			out = append(out, db.Lesson{Text: r.Line(), Task: r.Task})
		}
		return out
	}
	var order []int
	for i := range rules {
		if !keep[i] {
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if c := rules[a].Count - rules[b].Count; c != 0 {
			return c
		}
		return min(len(rules[a].from), 1) - min(len(rules[b].from), 1) // a new rule before a kept one
	})
	gone := map[int]bool{}
	kept := func() []Rule {
		var rs []Rule
		for i, r := range rules {
			if !gone[i] {
				rs = append(rs, r)
			}
		}
		return rs
	}
	for _, i := range order {
		if db.Fits(person, lessons(kept())) {
			break
		}
		gone[i] = true
		if len(rules[i].from) > 0 {
			dropped = append(dropped, rules[i])
		}
	}
	if !db.Fits(person, lessons(kept())) {
		return nil, nil, fmt.Errorf("a person's own rules alone are past the cap")
	}
	return kept(), dropped, nil
}
