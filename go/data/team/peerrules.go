package team

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The house rules before agents worked as peers (T9001) and after: MigratePeerRules turns the old parts of a rules file seeded from
// them into the new. Delete this file once every home has started a daemon that has it.
const (
	oldSendLine = "- Send the agent you report to no progress or FYI messages: it hears from you only when a task is done or you are blocked.\n"
	newSendLine = "- Send whoever opened your task no progress or FYI messages: it hears from you only when the task is done or you are blocked.\n"
	oldReports  = `If you have reports:
- You do not implement: give the work to a report and read what it sends back.
- One task at a time per report. Correct a task with a memo, not a message behind it.
- Record a wait as ` + "`--blocked-by`" + ` on the task, not in prose.
- Check main before you assign, so nobody rebuilds what landed.
- Quote the founder's own words in the task you open for their request, and answer the founder through ` + "`hl`" + `, never in chat.
- Finding the bottleneck is your job. At each periodic check, look at each report's queued count, its oldest wait and the work blocked behind it, and compare with the last check. When a wait passes 1h or a queue grows two checks in a row, ask that agent what slows it and open the work that fixes the cause. No dead or stalled agent does not mean healthy.
`
	newPeers = `Working with peers:
- Anyone gives anyone work as a task; correct a task you gave with a memo, not a message behind it.
- Record a wait as ` + "`--blocked-by`" + ` on the task, not in prose.
- Check main before you open work, so nobody rebuilds what landed.
- Quote the founder's own words in the task you open for their request, and answer the founder through ` + "`hl`" + `, never in chat.
- The contact passes work from another workspace to the peer it fits, and does its own work too.
`
)

// MigratePeerRules rewrites, once per workspace, the old send line and reports block of its rules file into the new ones, each only
// where it is there word for word: a part the team changed stays, and that file is named in kept. A workspace done before is skipped
// (its .peer-rules marker). A file that cannot be read or written is named in errs and the others go on.
func MigratePeerRules(home string) (kept []string, errs []error) {
	files, err := filepath.Glob(filepath.Join(home, "workspaces", "*", "base.md"))
	if err != nil {
		return nil, []error{err}
	}
	for _, f := range files {
		marker := filepath.Join(filepath.Dir(f), ".peer-rules")
		if _, err := os.Stat(marker); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		left, err := migratePeerFile(f)
		if err == nil {
			err = os.WriteFile(marker, nil, 0o644)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f, err))
			continue
		}
		if left {
			kept = append(kept, f)
		}
	}
	return kept, errs
}

// migratePeerFile is MigratePeerRules for one file; left is whether an old part stays in it, changed by its team.
func migratePeerFile(f string) (left bool, err error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return false, err
	}
	text := string(b)
	out := strings.Replace(strings.Replace(text, oldReports, newPeers, 1), oldSendLine, newSendLine, 1)
	left = strings.Contains(out, "If you have reports:") || strings.Contains(out, "the agent you report to")
	if out == text {
		return left, nil
	}
	// a new file renamed over the old: a crash leaves either, never an empty file that SeedRules would not fill again
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return left, err
	}
	return left, os.Rename(tmp, f)
}
