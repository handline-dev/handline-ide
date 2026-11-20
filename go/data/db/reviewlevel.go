package db

import (
	"database/sql"
	"errors"
	"path"
	"slices"
	"strings"

	"github.com/handline/handline/go/kit/gitrun"
)

// The review levels: how many of an agent's finished tasks of one kind of work the founder looks at.
const (
	LevelEvery      = "every"        // each one is Ready for the founder (the default)
	LevelOneInTwo   = "one_in_two"   // one in two is; the others take an automatic OK
	LevelOneInThree = "one_in_three" // one in three is
	LevelOneInFive  = "one_in_five"  // one in five is
	LevelNever      = "never"        // none is, but for a finish that may not skip the founder
)

// Levels are the levels from the most looked at to the least.
var Levels = []string{LevelEvery, LevelOneInTwo, LevelOneInThree, LevelOneInFive, LevelNever}

// LevelEach is the n of "1 in n" for level: 1 for every, 0 for never.
func LevelEach(level string) int {
	return map[string]int{LevelEvery: 1, LevelOneInTwo: 2, LevelOneInThree: 3, LevelOneInFive: 5}[level]
}

// The kinds of work a task's changed files make it, each with its own level.
const (
	WorkContract = "contract" // contract and data: the contract, migrations, a store's schema; always every
	WorkScreen   = "screen"   // the apps and the app packages
	WorkDaemon   = "daemon"   // other code
	WorkDocs     = "docs"     // docs, markdown, or a change of only deletions
)

// Works are the kinds of work from the most looked at by default to the least.
var Works = []string{WorkContract, WorkScreen, WorkDaemon, WorkDocs}

// WorkOf is the kind of work a change to the file at p is.
func WorkOf(p string) string {
	switch top, _, _ := strings.Cut(p, "/"); {
	case top == "contract" || slices.Contains(strings.Split(path.Dir(p), "/"), "migrations") ||
		path.Ext(p) == ".sql" && strings.Contains(path.Base(p), "schema"):
		return WorkContract
	case top == "apps" || top == "packages":
		return WorkScreen
	case top == "docs" || path.Ext(p) == ".md":
		return WorkDocs
	}
	return WorkDaemon
}

// WorksOf are the kinds of work a task that changed files is, in Works order: docs alone for only deletions outside contract and data,
// none for no files.
func WorksOf(files []gitrun.TaskFile) []string {
	seen := map[string]bool{}
	onlyDeleted := len(files) > 0
	for _, f := range files {
		seen[WorkOf(f.Path)] = true
		onlyDeleted = onlyDeleted && f.Deleted
	}
	if onlyDeleted && !seen[WorkContract] {
		return []string{WorkDocs}
	}
	var out []string
	for _, w := range Works {
		if seen[w] {
			out = append(out, w)
		}
	}
	return out
}

// AutoVerdictBy is who an automatic OK is by: no person, so the record leaves it out.
const AutoVerdictBy = "auto"

// LevelAt is an agent's review level at a kind of work now, the row that set it (0 for every by default) and the newest event then.
type LevelAt struct {
	Work  string
	Level string
	Row   int64
	After int64
}

// ReviewLevel is agent's level at work under model: every for contract, for no row, or for a row set under another model.
func ReviewLevel(q Q, agent, work, model string) (LevelAt, error) {
	l := LevelAt{Work: work, Level: LevelEvery}
	if work == WorkContract {
		return l, nil
	}
	var rowModel string
	err := q.QueryRow("SELECT level, id, after_event, model FROM review_levels WHERE agent = ? AND work = ? ORDER BY id DESC LIMIT 1",
		agent, work).Scan(&l.Level, &l.Row, &l.After, &rowModel)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return LevelAt{Work: work, Level: LevelEvery}, nil
	case err == nil && rowModel != model:
		return LevelAt{Work: work, Level: LevelEvery}, nil
	}
	return l, err
}

// Strictest is the most looked-at of levels: a task of several kinds of work is looked at as the strictest of them asks; every for none.
func Strictest(levels []LevelAt) LevelAt {
	if len(levels) == 0 {
		return LevelAt{Level: LevelEvery}
	}
	return slices.MinFunc(levels, func(a, b LevelAt) int { return slices.Index(Levels, a.Level) - slices.Index(Levels, b.Level) })
}

// SetReviewLevel records agent's level at work while it runs model, by `by`, why (set or redo), on task when a Redo moved it (0 for none).
func SetReviewLevel(q Q, agent, work, model, level, by, why string, task int64) error {
	var t any
	if task != 0 {
		t = task
	}
	_, err := q.Exec("INSERT INTO review_levels(agent, work, model, level, at, by, why, task, after_event) "+
		"VALUES(?, ?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(id), 0) FROM events))", agent, work, model, level, Now(), by, why, t)
	return err
}

// LevelDown is the level one step less looked at, "" from never.
func LevelDown(level string) string {
	for i, l := range Levels {
		if l == level && i+1 < len(Levels) {
			return Levels[i+1]
		}
	}
	return ""
}

// Look is one of an agent's tasks the founder gave a verdict on: whether the first was OK, and when.
type Look struct {
	Task    int64
	OKFirst bool
	At      int64
}

// readyOf is the SQL condition that a Ready event r was made by agent (?), running model (?), of work (?).
const readyOf = "r.agent = ? AND json_extract(r.payload, '$.model') = ? AND EXISTS (SELECT 1 FROM json_each(r.payload, '$.works') WHERE value = ?)"

// ReviewRecord is agent's looks at level l (ReviewLevel) while it runs model, oldest first: each task it made Ready of l's work whose first
// verdict by a person came after the level was set, and whether that verdict was OK. Automatic OKs are no look.
func ReviewRecord(q Q, agent, model string, l LevelAt) ([]Look, error) {
	// Only tasks with a verdict after the level can hold a look, so the first verdicts are found among those. Whose look it is: who made the
	// Ready the verdict answered, since an OK hands the task on to its land_by agent.
	rows, err := q.Query("SELECT f.task, json_extract(e.payload, '$.verdict'), e.ts FROM events e JOIN ("+
		"SELECT "+eventTask+" AS task, MIN(id) AS first FROM events WHERE kind = ? AND agent != ? AND "+eventTask+" IN ("+
		"SELECT "+eventTask+" FROM events WHERE kind = ? AND id > ?) GROUP BY 1) f ON e.id = f.first "+
		"JOIN events r ON r.id = (SELECT id FROM events WHERE kind = ? AND "+eventTask+" = f.task AND id < f.first ORDER BY id DESC LIMIT 1) "+
		"WHERE f.first > ? AND "+readyOf+" ORDER BY e.id",
		VerdictKind, AutoVerdictBy, VerdictKind, l.After, ReadyKind, l.After, agent, model, l.Work)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Look
	for rows.Next() {
		var k Look
		var verdict string
		if err := rows.Scan(&k.Task, &verdict, &k.At); err != nil {
			return nil, err
		}
		k.OKFirst = verdict == VerdictOK
		out = append(out, k)
	}
	return out, rows.Err()
}

// StepAfter is how many first-look OKs in a row at a level offer the founder the next level down.
const StepAfter = 10

// LearnedEnough is whether a record at l says the agent may be looked at less at l's work: not at never nor contract and data, and its
// last StepAfter looks all OK at first. ok is how many of its last StepAfter were.
func LearnedEnough(l LevelAt, record []Look) (yes bool, ok int) {
	for _, k := range record[max(0, len(record)-StepAfter):] {
		if k.OKFirst {
			ok++
		}
	}
	return l.Level != LevelNever && l.Work != WorkContract && ok == StepAfter, ok
}

// AgentWorks are the kinds of work agent has finished a task of or has a level at, in Works order.
func AgentWorks(q Q, agent string) ([]string, error) {
	rows, err := q.Query("SELECT j.value FROM events r, json_each(r.payload, '$.works') j WHERE r.kind = ? AND r.agent = ? "+
		"UNION SELECT work FROM review_levels WHERE agent = ?", ReadyKind, agent, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		seen[w] = true
	}
	var out []string
	for _, w := range Works {
		if seen[w] {
			out = append(out, w)
		}
	}
	return out, rows.Err()
}
