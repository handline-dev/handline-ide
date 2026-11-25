package rulesfold

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
)

// By is who the fold's lessons are by.
const By = "rules_fold"

const (
	// MaxRules is how many rules of each area the model is asked to keep at most; the bytes each area may take are db.AreaCap.
	MaxRules = 40
	// Budget is the most bytes of items one step sends.
	Budget   = 45000
	maxText  = 120 // the prompt asks for 80
	maxQuote = 90  // the prompt asks for 60
)

var (
	//go:embed prompt.md
	promptText string
	//go:embed schema.json
	Schema string
)

// Model answers prompt with JSON that matches schema.
type Model func(ctx context.Context, prompt, schema string) ([]byte, error)

// Result is what one step did: the items it read, the rule set after them, and whether it was written.
type Result struct {
	Items   int
	Rules   []Rule
	Dropped []Rule
	Next    Cursors
	Written bool
	// More is whether more was there to read than this step took.
	More bool
}

// kept is one of the person's rules now, and whether it is a person's: one a person or a Redo wrote, which the fold never drops.
type kept struct {
	Rule
	Text string // the lesson's text as stored
	keep bool
}

type outRule struct {
	Rule
	Merges []int64  `json:"merges"`
	Refs   []string `json:"refs"`
}

// Step folds person's next batch of words into their rules: it reads past the cursors, asks model, validates the answer and, when write,
// writes the rules and moves the cursors in one transaction. A batch of only acks moves the cursors and asks nothing. When the rules or
// cursors changed while the model answered (a Redo added a lesson), nothing is written: the next step reads the batch again.
func Step(ctx context.Context, d *sql.DB, person string, model Model, kind func(task int64) string, write bool) (Result, error) {
	cur, err := ReadCursors(d, person)
	if err != nil {
		return Result{}, err
	}
	b, err := ReadBatch(d, person, cur, Budget, kind)
	if err != nil || b.Next == cur {
		return Result{Next: cur}, err
	}
	before, err := keptRules(d, person)
	if err != nil {
		return Result{}, err
	}
	res := Result{Items: len(b.Items), Next: b.Next, More: b.Full}
	if len(b.Items) == 0 {
		for _, k := range before {
			r := k.Rule
			r.stored, r.from = k.Text, []int64{k.ID}
			res.Rules = append(res.Rules, r)
		}
	} else {
		raw, err := model(ctx, Prompt(before, b.Items), Schema)
		if err != nil {
			return res, err
		}
		var out struct {
			Rules []outRule `json:"rules"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return res, fmt.Errorf("the model's answer: %w", err)
		}
		if res.Rules, res.Dropped, err = check(person, before, out.Rules, b.Items); err != nil {
			return res, err
		}
	}
	if !write {
		return res, nil
	}
	err = inTx(d, func(tx *sql.Tx) error {
		now, err := ReadCursors(tx, person)
		if err != nil {
			return err
		}
		again, err := keptRules(tx, person)
		if err != nil {
			return err
		}
		if now != cur || !sameRules(before, again) {
			return ErrMoved
		}
		if err := apply(tx, person, res); err != nil {
			return err
		}
		return writeCursors(tx, person, b.Next)
	})
	res.Written = err == nil
	return res, err
}

// ErrMoved is a step whose rules or cursors changed while the model answered.
var ErrMoved = errors.New("the rules changed while the model read them; the next step reads the batch again")

func inTx(d *sql.DB, f func(*sql.Tx) error) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// keptRules are person's live lessons as rules, each marked keep when a person or a Redo wrote it or any lesson merged into it.
func keptRules(q db.Q, person string) ([]kept, error) {
	live, err := db.LiveLessons(q, person)
	if err != nil {
		return nil, err
	}
	out := make([]kept, 0, len(live))
	for _, l := range live {
		var others int
		err := q.QueryRow(`WITH RECURSIVE src(id) AS (SELECT ? UNION SELECT l.id FROM lessons l JOIN src ON l.merged_into = src.id)
			SELECT COUNT(*) FROM lessons WHERE id IN (SELECT id FROM src) AND created_by != ?`, l.ID, By).Scan(&others)
		if err != nil {
			return nil, err
		}
		out = append(out, kept{Rule: FromLesson(l), Text: l.Text, keep: others > 0})
	}
	return out, nil
}

func sameRules(a, b []kept) bool {
	return slices.EqualFunc(a, b, func(x, y kept) bool { return x.ID == y.ID && x.Text == y.Text })
}

// Prompt is the model's input: the instructions, the rules kept so far and the items.
func Prompt(rules []kept, items []Item) string {
	type shown struct {
		Rule
		Keep bool `json:"keep,omitempty"`
	}
	rs := make([]shown, 0, len(rules))
	for _, k := range rules {
		rs = append(rs, shown{k.Rule, k.keep})
	}
	r, _ := json.Marshal(rs)
	it, _ := json.Marshal(items)
	return strings.ReplaceAll(promptText, "MAXRULES", strconv.Itoa(MaxRules)) + "\nRULES:\n" + string(r) + "\n\nITEMS:\n" + string(it) + "\n"
}
