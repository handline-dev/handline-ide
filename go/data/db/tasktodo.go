package db

import (
	"encoding/json"
	"fmt"
)

// TaskTodo is the step count the working agent last gave with its progress note on a task (hl progress --done N --of M "note"): what
// task.get answers as todo.
type TaskTodo struct {
	By        string `json:"by"` // the agent that gave it: a count of another agent's is not the working one's
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Current   string `json:"current"`
	UpdatedAt int64  `json:"updated_at"`
}

func taskTodoKey(tid int64) string { return fmt.Sprintf("progress-todo:%d", tid) }

// SetTaskTodo keeps the count on the task, stamped now; nil clears it (a note with no count).
func SetTaskTodo(q Q, tid int64, todo *TaskTodo) error {
	if todo == nil {
		return ClearMeta(q, taskTodoKey(tid))
	}
	t := *todo
	t.UpdatedAt = Now()
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return SetMeta(q, taskTodoKey(tid), string(b))
}

// ReadTaskTodo is the count kept on the task, nil when none is.
func ReadTaskTodo(q Q, tid int64) (*TaskTodo, error) {
	v, ok, err := Meta(q, taskTodoKey(tid))
	if err != nil || !ok {
		return nil, err
	}
	var t TaskTodo
	if err := json.Unmarshal([]byte(v), &t); err != nil {
		return nil, nil // a row this code did not write is no count
	}
	return &t, nil
}
