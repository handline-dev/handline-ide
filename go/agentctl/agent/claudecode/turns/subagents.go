package turns

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// watchSubagents reads, while a turn is watched, each subagent transcript the session writes beside its own (<session>/subagents/
// agent-<id>.jsonl, its tool call named in the .meta.json next to it) and hands its lines to SubLines: from where the last turn's watch
// stopped, so a line is told once, and a subagent that wrote between turns is read at the next.
func (t *Tracker) watchSubagents(agent, session, path string) (stop func()) {
	dir := filepath.Join(strings.TrimSuffix(path, filepath.Ext(path)), "subagents")
	every := t.retryWait
	if every == 0 {
		every = transcriptRetry
	}
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tails := map[string]*tail{}
		told := map[string]func([]byte){}
		defer func() {
			for _, tl := range tails {
				tl.f.Close()
			}
		}()
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			files, _ := filepath.Glob(filepath.Join(dir, "agent-*.jsonl"))
			for _, f := range files {
				if _, ok := tails[f]; !ok {
					parent := subagentCall(strings.TrimSuffix(f, ".jsonl") + ".meta.json")
					if parent == "" {
						continue // its meta is not written yet
					}
					fh, err := os.Open(f)
					if err != nil {
						continue
					}
					if _, err := fh.Seek(t.subOffset(f), io.SeekStart); err != nil {
						fh.Close()
						continue
					}
					tails[f] = &tail{f: fh}
					told[f] = t.SubLines(agent, session, parent)
				}
				if fn := told[f]; fn != nil {
					tails[f].read(fn)
				}
				if off, err := tails[f].f.Seek(0, io.SeekCurrent); err == nil {
					t.setSubOffset(f, off-int64(len(tails[f].partial)))
				}
			}
			select {
			case <-quit:
				return
			case <-tick.C:
			}
		}
	}()
	return func() {
		close(quit)
		<-done
	}
}

// subagentCall is the tool call that started a subagent, from its meta file; "" when it cannot be read.
func subagentCall(meta string) string {
	b, err := os.ReadFile(meta)
	if err != nil {
		return ""
	}
	var m struct {
		ToolUseID string `json:"toolUseId"`
	}
	_ = json.Unmarshal(b, &m)
	return m.ToolUseID
}

// subOffset is how far f has been told, kept across turns.
func (t *Tracker) subOffset(f string) int64 {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	return t.subRead[f]
}

func (t *Tracker) setSubOffset(f string, off int64) {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	if t.subRead == nil {
		t.subRead = map[string]int64{}
	}
	t.subRead[f] = off
}
