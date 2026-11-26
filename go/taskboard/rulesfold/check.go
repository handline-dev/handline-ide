package rulesfold

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
)

// check validates the model's rule set against the rules kept before and the items it read, and gives the rules to write and the kept
// ones it drops. An id or merge that names no kept rule is left out: a new rule. A kept rule the model does not return stays as it was:
// a long list is not trusted to come back whole. A rule's count is the counts it had and one for each item read its refs name. A quote
// names an item read or the quote it had. A rule that comes back as it was kept is not checked again, so a person's long rule stands as
// they wrote it. Past the cap the fold's own rules go, the least said first and new ones before kept ones, until they fit; a person's
// never go, and a set that cannot fit is refused.
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
	byText := map[string]int64{}
	for _, k := range before {
		byText[k.Rule.Text] = k.ID
	}
	for _, o := range out {
		r := o.Rule
		if _, ok := old[r.ID]; !ok {
			r.ID = byText[r.Text] // a kept rule's words without its id are that rule; else a new rule
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
		said := map[string]bool{}
		for _, ref := range o.Refs {
			if _, ok := byRef[ref]; ok {
				said[ref] = true
			}
		}
		r.Count = max(prior+len(said), 1) // what it had, and one for each item read that says it: no count of the model's
		if was != nil && same(was.Rule, r) {
			r.Task, r.stored = was.Task, was.Text
		} else {
			if r.QuoteRef == "" && was != nil { // a rule said again keeps the quote it had
				r.Quote, r.QuoteRef = was.Quote, was.QuoteRef
			}
			// a person's own rule keeps its words however long; only the model's are held to the limits
			if err := checkText(r, was == nil || !was.keep || r.Text != was.Rule.Text); err != nil {
				return nil, nil, err
			}
			if it, ok := byRef[r.QuoteRef]; ok {
				r.Task = it.task
			} else if r.QuoteRef != "" && !quotes[r.QuoteRef] { // no quote is a rule kept without one
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
		r := k.Rule
		r.stored, r.from = k.Text, []int64{k.ID}
		if k.keep {
			keep[len(rules)] = true
		}
		rules = append(rules, r)
	}
	return fit(person, rules, keep, dropped)
}

// checkText checks the quote of r, and its text when text.
func checkText(r Rule, text bool) error {
	switch {
	case text && (r.Text == "" || utf8.RuneCountInString(r.Text) > maxText || strings.ContainsAny(r.Text, "\n\r")):
		return fmt.Errorf("rule text %q is empty, over %d characters or more than a line", r.Text, maxText)
	case r.QuoteRef != "" && (r.Quote == "" || utf8.RuneCountInString(r.Quote) > maxQuote || strings.ContainsAny(r.Quote, "\n\r")):
		return fmt.Errorf("rule %q has a quote that is empty, over %d characters or more than a line", r.Text, maxQuote)
	}
	return nil
}

// fit drops the fold's own rules, the least said first and new ones before kept ones, until they fit the cap and number at most
// MaxRules. A dropped rule that replaces kept ones takes them with it.
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
		if rs := kept(); len(rs) <= MaxRules && db.Fits(person, lessons(rs)) {
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
