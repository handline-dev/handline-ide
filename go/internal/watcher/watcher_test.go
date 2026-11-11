package watcher

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/kit/storetest"
)

func rig(t *testing.T) (*sql.DB, *stream.Log) {
	t.Helper()
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, &stream.Log{DB: d}
}

func exec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := d.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func rows(t *testing.T, l *stream.Log) []*stream.Event {
	t.Helper()
	got, err := l.Since(0, []string{"tasks"}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTheFirstLookSaysNothingAndEachChangeIsOneTaskChangedRow(t *testing.T) {
	d, l := rig(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'old', 'bob', 'p1', 1)")
	tk := &Tasks{Reads: &reads.Env{DB: d}}
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	if err := tk.Scan(d, emit); err != nil || len(rows(t, l)) != 0 {
		t.Fatalf("the first look said something: %v %v", err, rows(t, l))
	}
	if err := tk.Scan(d, emit); err != nil || len(rows(t, l)) != 0 {
		t.Fatalf("nothing changed: %v", rows(t, l))
	}
	exec(t, d, "UPDATE tasks SET title = 'new' WHERE id = 1")
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'second', 'carol', 'p1', 1)")
	if err := tk.Scan(d, emit); err != nil {
		t.Fatal(err)
	}
	got := rows(t, l)
	if len(got) != 2 || got[0].Type != "task.changed" || *got[0].Agent != "bob" || *got[0].Title != "new" || string(got[0].Target) != `{"id":"1","screen":"tasks"}` {
		t.Fatalf("%+v", got)
	}
	var task map[string]any
	if json.Unmarshal(got[0].Data, &task) != nil || task["id"] != float64(1) || task["to"] != "bob" || task["state"] != "draft" || *got[1].Agent != "carol" {
		t.Fatalf("%v", task)
	}
	exec(t, d, "INSERT INTO task_memos(task_id, author, body, created_at) VALUES(1, 'bob', 'note', 5)")
	if err := tk.Scan(d, emit); err != nil || len(rows(t, l)) != 3 {
		t.Fatalf("a memo is a change: %v %d", err, len(rows(t, l)))
	}
}

func TestAnEndedTaskLeavesTheWatchAfterADayWithoutAnEvent(t *testing.T) {
	d, l := rig(t)
	now := time.Now()
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence, done_at) VALUES(1, 't', 'bob', 'p1', 1, 'e', ?)", now.Unix()-10)
	tk := &Tasks{Reads: &reads.Env{DB: d}, Now: func() time.Time { return now }}
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	tk.Scan(d, emit)
	tk.Now = func() time.Time { return now.Add(2 * 24 * time.Hour) }
	if err := tk.Scan(d, emit); err != nil || len(rows(t, l)) != 0 {
		t.Fatalf("%v %v", err, rows(t, l))
	}
}

// scanned counts its Scan calls: the first one is the watcher's first look.
type scanned struct{ n atomic.Int32 }

func (*scanned) Name() string { return "scanned" }
func (s *scanned) Scan(*sql.DB, Emit) error {
	s.n.Add(1)
	return nil
}

// firstLook waits for the watcher's first scan, and fails if none comes.
func firstLook(t *testing.T, s *scanned) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); s.n.Load() == 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the watcher never made its first look")
		}
	}
}

type failing struct{}

func (failing) Name() string             { return "failing" }
func (failing) Scan(*sql.DB, Emit) error { return sql.ErrConnDone }

func TestAFailingDetectorDoesNotStopTheOthersAndTheWatchSeesAnotherConnectionsCommit(t *testing.T) {
	d, l := rig(t)
	var mu sync.Mutex
	var logged []string
	look := &scanned{}
	w := &Watcher{DB: d, Emit: func(e stream.Entry) (*stream.Event, error) { return l.Append(e) },
		Detectors: []Detector{failing{}, &Tasks{Reads: &reads.Env{DB: d}}, look},
		Log:       func(f string, a ...any) { mu.Lock(); logged = append(logged, f); mu.Unlock() }}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { w.Run(stop); close(done) }()
	firstLook(t, look)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'bob', 'p1', 1)")
	for i := 0; i < 100 && len(rows(t, l)) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(Every + Every/2) // a negative: no second row comes
	close(stop)
	<-done
	mu.Lock()
	defer mu.Unlock()
	if n := len(rows(t, l)); n != 1 || len(logged) == 0 {
		t.Fatalf("%d rows, %d failures logged", n, len(logged))
	}
}

func TestPrimeStartsANewCursorAtTheNewestRowAndKeepsAnOldOne(t *testing.T) {
	d, _ := rig(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(7, 't', 'bob', 'p1', 1)")
	got, err := Prime(d, "stream:test", "SELECT COALESCE(MAX(id), 0) FROM tasks")
	if err != nil || got != 7 {
		t.Fatalf("%d %v", got, err)
	}
	if err := db.SetCursor(d, "stream:test", 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := Prime(d, "stream:test", "SELECT 99"); got != 3 {
		t.Fatalf("a cursor that is set moved: %d", got)
	}
}

// Reading a task's comments changes its unread count, so every device is told: the marker is a change of the task.
func TestTheFoundersReadMarkerIsAChangeOfTheTask(t *testing.T) {
	d, l := rig(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'old', 'bob', 'p1', 1)")
	exec(t, d, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(4, 1, 'bob', 'note', 5)")
	tk := &Tasks{Reads: &reads.Env{DB: d}}
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	if err := tk.Scan(d, emit); err != nil {
		t.Fatal(err)
	}
	exec(t, d, "INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:human:1', 4, 6)")
	if err := tk.Scan(d, emit); err != nil {
		t.Fatal(err)
	}
	got := rows(t, l)
	var task map[string]any
	if len(got) != 1 || json.Unmarshal(got[0].Data, &task) != nil || task["unread"] != float64(0) {
		t.Fatalf("the marker is one task.changed row that says nothing is unread: %+v %v", got, task)
	}
}
