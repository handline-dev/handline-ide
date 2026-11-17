package db

import (
	"database/sql"
	"errors"
)

// The review levels: how many of an agent's finished story sub-tasks the founder looks at.
const (
	LevelEvery      = "every"        // each one is Ready for the founder (the default)
	LevelOneInThree = "one_in_three" // one in three is; the others take an automatic OK
	LevelAuto       = "auto"         // none is, but for a finish that may not skip the founder
)

// Levels are the levels from the most looked at to the least.
var Levels = []string{LevelEvery, LevelOneInThree, LevelAuto}

// AutoVerdictBy is who an automatic OK is by: no person, so the record leaves it out.
const AutoVerdictBy = "auto"

// LevelAt is an agent's review level now, the row that set it (0 for the default, every, which no row set) and the newest event then.
type LevelAt struct {
	Level string
	Row   int64
	After int64
}

// ReviewLevel is agent's level now.
func ReviewLevel(q Q, agent string) (LevelAt, error) {
	l := LevelAt{Level: LevelEvery}
	err := q.QueryRow("SELECT level, id, after_event FROM review_levels WHERE agent = ? ORDER BY id DESC LIMIT 1", agent).Scan(&l.Level, &l.Row, &l.After)
	if errors.Is(err, sql.ErrNoRows) {
		return LevelAt{Level: LevelEvery}, nil
	}
	return l, err
}

// SetReviewLevel records agent's level, by `by`, why (set or redo), on task when a Redo moved it (0 for none).
func SetReviewLevel(q Q, agent, level, by, why string, task int64) error {
	var t any
	if task != 0 {
		t = task
	}
	_, err := q.Exec("INSERT INTO review_levels(agent, level, at, by, why, task, after_event) "+
		"VALUES(?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(id), 0) FROM events))", agent, level, Now(), by, why, t)
	return err
}

// LevelUp is the level one step more looked at than level; every stays every.
func LevelUp(level string) string {
	for i, l := range Levels {
		if l == level && i > 0 {
			return Levels[i-1]
		}
	}
	return LevelEvery
}

// LevelDown is the level one step less looked at, "" from auto.
func LevelDown(level string) string {
	for i, l := range Levels {
		if l == level && i+1 < len(Levels) {
			return Levels[i+1]
		}
	}
	return ""
}

// Look is one of an agent's story sub-tasks the founder gave a verdict on: whether the first was OK, and when.
type Look struct {
	Task    int64
	OKFirst bool
	At      int64
}

// ReviewRecord is agent's looks at level l (ReviewLevel), oldest first: each task it made Ready whose first verdict by a person came after
// the level was set, and whether that verdict was OK. Automatic OKs are no look.
func ReviewRecord(q Q, agent string, l LevelAt) ([]Look, error) {
	// Only tasks with a verdict after the level can hold a look, so the first verdicts are found among those. Whose look it is: who made the
	// Ready the verdict answered, since an OK hands the task on to its land_by agent.
	rows, err := q.Query("SELECT f.task, json_extract(e.payload, '$.verdict'), e.ts FROM events e JOIN ("+
		"SELECT "+eventTask+" AS task, MIN(id) AS first FROM events WHERE kind = ? AND agent != ? AND "+eventTask+" IN ("+
		"SELECT "+eventTask+" FROM events WHERE kind = ? AND id > ?) GROUP BY 1) f ON e.id = f.first WHERE f.first > ? AND "+
		"(SELECT agent FROM events WHERE kind = ? AND "+eventTask+" = f.task AND id < f.first ORDER BY id DESC LIMIT 1) = ? ORDER BY e.id",
		VerdictKind, AutoVerdictBy, VerdictKind, l.After, l.After, ReadyKind, agent)
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

// LearnedEnough is whether a record at level says the agent may be looked at less: not at auto, and 8 of its last 10 looks OK at first.
// ok is how many of the last 10 were.
func LearnedEnough(level string, record []Look) (yes bool, ok int) {
	for _, k := range record[max(0, len(record)-10):] {
		if k.OKFirst {
			ok++
		}
	}
	return level != LevelAuto && len(record) >= 10 && ok >= 8, ok
}
