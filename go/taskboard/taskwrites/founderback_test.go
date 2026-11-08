package taskwrites

import (
	"fmt"
	"testing"
)

func TestFounderTasksGoBackToWhoHandedThemOver(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sdb, _, _ := reportEnvIn(t, home,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'handed', 'human', 'boss', 1), (2, 'opened', 'human', 'w', 1), "+
			"(3, 'ask', 'human', 'human', 1), (4, 'notice', 'human', 'task-board', 1), (5, 'done', 'human', 'boss', 1)",
		"UPDATE tasks SET done_evidence = 'ok' WHERE id = 5",
		`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'w', 'task_handoff', '{"task_id":1,"from":"w","to":"human","note":"which one?"}'), `+
			`(2, 'gate', 'task_handoff', '{"task_id":1,"from":"gate","to":"human","note":"later"}')`,
	)
	said, err := ReturnFounderTasks(sdb, home)
	if err != nil || fmt.Sprint(said) != "[T1 goes back to w, the founder's turn on it T2 goes back to w, the founder's turn on it]" {
		t.Fatalf("%q %v", said, err)
	}
	for id, want := range map[int]int{1: 1, 2: 1, 3: 0, 4: 0, 5: 0} {
		if got := count(t, sdb, fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE id = %d AND owner = 'w'", id)); got != want {
			t.Errorf("T%d: owner w is %d, want %d", id, got, want)
		}
	}
	if count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1 AND agent = 'human' AND pushed_by = 'w' AND ask = 'which one?'") != 1 ||
		count(t, sdb, "SELECT COUNT(*) FROM requests WHERE target = 'human' AND state = 'queued'") != 0 {
		t.Errorf("each goes back with the founder's turn on it asking the note, its owner under it")
	}
	if count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 2 AND agent = 'human' AND ask = 'opened'") != 1 {
		t.Errorf("a task handed over with no note asks with its title")
	}
	sdb.Exec("UPDATE tasks SET owner = 'human' WHERE id = 2") // the founder may take a task, and it stays theirs after a restart
	if again, err := ReturnFounderTasks(sdb, home); err != nil || len(again) != 0 {
		t.Fatalf("it runs once: %q %v", again, err)
	}
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = 2 AND owner = 'human'") != 1 {
		t.Fatal("a task the founder took stays theirs")
	}
}
