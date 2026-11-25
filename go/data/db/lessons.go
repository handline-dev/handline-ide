package db

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/handline/handline/go/kit/actor"
)

// LessonsCap is the most bytes a section of lessons takes in a launch prompt (Section): an agent's own, or each area of a person's (AreaCap);
// an add past it is refused, and a merge brings them back under it.
const LessonsCap = 3072

// The areas a person's rules fall in: how a screen looks and behaves, and every other kind of work. A rule's area is the start of its
// text ("Screen: ..."); a rule without one is a work rule.
const (
	AreaScreen = "screen"
	AreaWork   = "work"
)

// screenPrefix starts the text of a person's screen rule.
const (
	screenPrefix = "Screen: "
	workPrefix   = "Work: "
)

// AreaCap is the most bytes each area of a person's rules keeps, counted as a section of its own: room for more than a launch prompt
// shows, so a rule said less often can still rise.
var AreaCap = map[string]int{AreaScreen: 12 << 10, AreaWork: 12 << 10}

// LessonArea is the area of a person's rule of text.
func LessonArea(text string) string {
	if strings.HasPrefix(text, screenPrefix) {
		return AreaScreen
	}
	return AreaWork
}

// AreaText is a person's rule text in area, starting with the area's name.
func AreaText(area, text string) string {
	if area == AreaScreen {
		return screenPrefix + text
	}
	return workPrefix + text
}

// Fits is whether lessons of agent's stay within the cap: an agent's section under LessonsCap, a person's each area under its AreaCap.
func Fits(agent string, lessons []Lesson) bool {
	if !actor.IsPerson(agent) {
		return len(Section(agent, lessons)) <= LessonsCap
	}
	by := map[string][]Lesson{}
	for _, l := range lessons {
		a := LessonArea(l.Text)
		by[a] = append(by[a], l)
	}
	for a, ls := range by {
		if len(Section(agent, ls)) > AreaCap[a] {
			return false
		}
	}
	return true
}

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
	return Collect[Lesson](q.Query("SELECT id, agent, text, COALESCE(task, 0), created_at, created_by FROM lessons WHERE agent = ? AND removed_at IS NULL ORDER BY id", agent))(
		func(rows *sql.Rows) (l Lesson, err error) {
			return l, rows.Scan(&l.ID, &l.Agent, &l.Text, &l.Task, &l.CreatedAt, &l.CreatedBy)
		})
}

// LessonsSection is the launch prompt's part that carries an agent's own lessons, "" for none.
func LessonsSection(lessons []Lesson) string {
	return section("# What you learned (your lessons)\nFrom the founder's Redo notes. Apply them to every task.", lessons)
}

// FounderSection is the launch prompt's part that carries the founder's lessons (rows whose agent is their key), "" for none.
func FounderSection(lessons []Lesson) string {
	return section(founderHead, lessons)
}

const founderHead = "# The founder's preferences\nWhat the founder said they want, kept by the agents who work for them. Follow them on every task."

// Section is the launch prompt section lessons of agent's go in: a person's are the founder's preferences.
func Section(agent string, lessons []Lesson) string {
	if actor.IsPerson(agent) {
		return FounderSection(lessons)
	}
	return LessonsSection(lessons)
}

// ShowCap is the most bytes of each area of a person's rules a launch prompt shows: the most said first, a rule without a count as said
// once.
var ShowCap = map[string]int{AreaScreen: 3 << 10, AreaWork: 3 << 10}

var (
	saidTimes = regexp.MustCompile(` \(said (\d+) times`)
	saidTail  = regexp.MustCompile(` \(said (?:once|\d+ times)(?:; .*)?\)$`)
)

// saidCount is how many times a person's rule says it was said, 1 when it does not say.
func saidCount(text string) int {
	if m := saidTimes.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 1
}

// PromptSection is the launch prompt's section of agent's lessons: all of an agent's own; of a person's, each area's most said rules up
// to its ShowCap, in the order they were kept, each only its rule: how often it was said and where are for ranking, not for agents.
func PromptSection(agent string, lessons []Lesson) string {
	if !actor.IsPerson(agent) {
		return LessonsSection(lessons)
	}
	byCount := slices.Clone(lessons)
	// a rule a person wrote by hand (no "said" tail) goes first, then the most said
	slices.SortStableFunc(byCount, func(a, b Lesson) int {
		if ha, hb := !saidTail.MatchString(a.Text), !saidTail.MatchString(b.Text); ha != hb {
			if ha {
				return -1
			}
			return 1
		}
		return saidCount(b.Text) - saidCount(a.Text)
	})
	used, shown := map[string]int{}, map[int64]bool{}
	for _, l := range byCount {
		a := LessonArea(l.Text)
		n := len(section("", []Lesson{{Text: saidTail.ReplaceAllString(l.Text, "")}}))
		if used[a]+n > ShowCap[a] {
			continue
		}
		used[a] += n
		shown[l.ID] = true
	}
	by := map[string][]Lesson{}
	for _, l := range lessons {
		if shown[l.ID] {
			a := LessonArea(l.Text)
			text := saidTail.ReplaceAllString(l.Text, "")
			text = strings.TrimPrefix(strings.TrimPrefix(text, screenPrefix), workPrefix)
			by[a] = append(by[a], Lesson{Text: text})
		}
	}
	if len(by) == 0 {
		return ""
	}
	s := founderHead
	for _, a := range []string{AreaWork, AreaScreen} {
		s += section("\n## "+areaName[a], by[a])
	}
	return s
}

// areaName is an area as a launch prompt heads its rules.
var areaName = map[string]string{AreaWork: "Work", AreaScreen: "Screen"}

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
	if !Fits(agent, append(live, Lesson{Text: text, Task: task})) {
		return 0, ErrLessonsFull
	}
	return addLesson(q, agent, text, task, by)
}

// addLesson is AddLesson past the cap check, for a caller that checked the whole set it writes.
func addLesson(q Q, agent, text string, task int64, by string) (int64, error) {
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
	return mergeLessons(q, agent, live, ids, text, 0, by, true)
}

// Revision is one change to a set of lessons: text replaces the live lessons IDs (none for a new one), on task Task (0: a merge keeps its
// newest source's).
type Revision struct {
	IDs  []int64
	Text string
	Task int64
}

// ReviseLessons makes revs to agent's lessons at once, each id in at most one of them: refused with ErrLessonsFull when the lessons they
// leave are past the cap, whatever order they are written in.
func ReviseLessons(q Q, agent string, revs []Revision, by string) error {
	live, err := LiveLessons(q, agent)
	if err != nil {
		return err
	}
	gone := map[int64]bool{}
	var after []Lesson
	for _, r := range revs {
		for _, id := range r.IDs {
			if gone[id] {
				return fmt.Errorf("lesson %d is in two revisions", id)
			}
			gone[id] = true
		}
		after = append(after, Lesson{Text: r.Text, Task: r.Task})
	}
	for _, l := range live {
		if !gone[l.ID] {
			after = append(after, l)
		}
	}
	if !Fits(agent, after) {
		return ErrLessonsFull
	}
	for _, r := range revs {
		if len(r.IDs) == 0 {
			_, err = addLesson(q, agent, r.Text, r.Task, by)
		} else {
			_, err = mergeLessons(q, agent, live, r.IDs, r.Text, r.Task, by, false)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func mergeLessons(q Q, agent string, live []Lesson, ids []int64, text string, task int64, by string, capped bool) (int64, error) {
	gone := map[int64]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	var kept []Lesson
	given := task
	for _, l := range live {
		if gone[l.ID] {
			delete(gone, l.ID)
			if l.Task != 0 && given == 0 {
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
	if capped && !Fits(agent, append(kept, Lesson{Text: text, Task: task})) {
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
