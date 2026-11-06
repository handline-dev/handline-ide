package db

import "strconv"

// SeenFloorMeta holds the comment number at or below which a task with no marker of its own counts as read.
const SeenFloorMeta = "task_seen_floor"

func SeenKey(tid int64) string { return "task_seen:" + Human + ":" + strconv.FormatInt(tid, 10) }
