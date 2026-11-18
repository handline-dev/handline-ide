package db

import (
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/kit/actor"
)

// LessonsCap is the most bytes a section of lessons takes in a launch prompt (Section): an agent's own, or a person's; an add past it is
// refused, and a merge brings them back under it.
const LessonsCap = 3072

// LessonKind is the task event a lesson's add writes, for the task's chat to show what was learned on it (the app's part comes later).
const LessonKind = "lesson"

// Lesson is one thing an agent learned, from task Task when it names one (0 for none).
type Lesson struct {
	ID        int64
	Agent     string
	Text      string
	Task      int64
	CreatedAt int64
	CreatedBy string
}

// ErrLessonsFull is an add or merge that would take the lessons past LessonsCap.
var ErrLessonsFull = errors.New("lessons full")

// NotLiveLesson is a merge of a lesson that is not one of the agent's live ones.
type NotLiveLesson int64

func (n NotLiveLesson) Error() string {
	return fmt.Sprintf("lesson %d is not one of your live lessons", int64(n))
}

// LiveLessons are agent's lessons not removed or merged, oldest first.
func LiveLessons(q Q, agent string) ([]Lesson, error) {
	rows, err := q.Query("SELECT id, agent, text, COALESCE(task, 0), created_at, created_by FROM lessons WHERE agent = ? AND removed_at IS NULL ORDER BY id", agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.Agent, &l.Text, &l.Task, &l.CreatedAt, &l.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LessonsSection is the launch prompt's part that carries an agent's own lessons, "" for none.
func LessonsSection(lessons []Lesson) string {
	return section("# What you learned (your lessons)\nFrom the founder's Redo notes. Apply them to every task.", lessons)
}

// FounderSection is the launch prompt's part that carries the founder's lessons (rows whose agent is their key), "" for none.
func FounderSection(lessons []Lesson) string {
	return section("# The founder's preferences\nWhat the founder said they want, kept by the agents who work for them. Follow them on every task.", lessons)
}

// Section is the launch prompt section lessons of agent's go in: a person's are the founder's preferences.
func Section(agent string, lessons []Lesson) string {
	if actor.IsPerson(agent) {
		return FounderSection(lessons)
	}
	return LessonsSection(lessons)
}

func section(head string, lessons []Lesson) string {
	if len(lessons) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(head)
	for _, l := range lessons {
		b.WriteString("\n- " + l.Text)
		if l.Task != 0 {
			fmt.Fprintf(&b, " (T%d)", l.Task)
		}
	}
	return b.String()
}

// AddLesson adds a lesson of agent's (a person's: the founder's), by `by`, refused with ErrLessonsFull when the section would pass LessonsCap.
func AddLesson(q Q, agent, text string, task int64, by string) (int64, error) {
	live, err := LiveLessons(q, agent)
	if err != nil {
		return 0, err
	}
	if len(Section(agent, append(live, Lesson{Text: text, Task: task}))) > LessonsCap {
		return 0, ErrLessonsFull
	}
	id, err := insertLesson(q, agent, text, task, by)
	if err == nil && task != 0 {
		_, err = AddEvent(q, by, LessonKind, KV{"task_id", task}, KV{"lesson", id}, KV{"text", text})
	}
	return id, err
}

// MergeLessons replaces lessons ids, each a live one of agent's, with one of text: they are kept, marked merged into it. Its task is the
// newest source's that names one. Refused with ErrLessonsFull when even the merge leaves the section past LessonsCap.
func MergeLessons(q Q, agent string, ids []int64, text, by string) (int64, error) {
	live, err := LiveLessons(q, agent)
	if err != nil {
		return 0, err
	}
	gone := map[int64]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	var kept []Lesson
	var task int64
	for _, l := range live {
		if gone[l.ID] {
			delete(gone, l.ID)
			if l.Task != 0 {
				task = l.Task
			}
		} else {
			kept = append(kept, l)
		}
	}
	for _, id := range ids {
		if gone[id] {
			return 0, NotLiveLesson(id)
		}
	}
	if len(Section(agent, append(kept, Lesson{Text: text, Task: task}))) > LessonsCap {
		return 0, ErrLessonsFull
	}
	id, err := insertLesson(q, agent, text, task, by)
	if err != nil {
		return 0, err
	}
	for _, old := range ids {
		res, err := q.Exec("UPDATE lessons SET removed_at = ?, removed_by = ?, merged_into = ? WHERE id = ? AND agent = ? AND removed_at IS NULL",
			Now(), by, id, old, agent)
		if err != nil {
			return 0, err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return 0, errors.Join(err, NotLiveLesson(old))
		}
	}
	return id, nil
}

// RemoveLesson marks lesson id removed by `by`; false when there is no such live lesson.
func RemoveLesson(q Q, id int64, by string) (bool, error) {
	res, err := q.Exec("UPDATE lessons SET removed_at = ?, removed_by = ? WHERE id = ? AND removed_at IS NULL", Now(), by, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func insertLesson(q Q, agent, text string, task int64, by string) (int64, error) {
	var t any
	if task != 0 {
		t = task
	}
	res, err := q.Exec("INSERT INTO lessons(agent, text, task, created_at, created_by) VALUES(?, ?, ?, ?, ?)", agent, text, t, Now(), by)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
