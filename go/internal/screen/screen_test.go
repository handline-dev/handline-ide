package screen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

const listed = `{"access": true, "windows": [{"id": 7, "app": "Finder", "title": "Downloads"}, {"id": 9, "app": "Notes", "title": "x"}]}`

func env(t *testing.T, access bool, config string) (*Env, *[]string) {
	t.Helper()
	home := t.TempDir()
	if config != "" {
		os.WriteFile(filepath.Join(home, "config.yaml"), []byte(config), 0o644)
	}
	db, err := store.Open(filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var ran []string
	list := listed
	if !access {
		list = strings.Replace(list, "true", "false", 1)
	}
	return &Env{DB: db, Home: home, Now: func() time.Time { return time.Unix(1700000000, 0) }, Run: func(name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args[:min(len(args), 2)], " "))
		if name == "screencapture" {
			return nil, os.WriteFile(args[len(args)-1], []byte("png"), 0o644)
		}
		return []byte(list), nil
	}}, &ran
}

func events(t *testing.T, db *sql.DB) []string {
	var all string
	if err := db.QueryRow(`SELECT COALESCE(group_concat(agent || ' ' || payload, char(10)), '') FROM events WHERE kind = ?`, Event).Scan(&all); err != nil {
		t.Fatal(err)
	}
	if all == "" {
		return nil
	}
	return strings.Split(all, "\n")
}

func shot(e *Env, p map[string]any) (any, error) {
	return e.shot(&rpc.Conn{}, p)
}

func TestShotSavesTheWindowAndLeavesOneEvent(t *testing.T) {
	e, ran := env(t, true, "")
	got, err := shot(e, map[string]any{"agent": "a1", "app": "finder"})
	if err != nil {
		t.Fatal(err)
	}
	path := got.(map[string]any)["path"].(string)
	if !strings.HasPrefix(path, filepath.Join(e.Home, "shots", "a1")) || !strings.HasSuffix(path, "Finder.png") {
		t.Fatalf("path %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(*ran, "|"), "screencapture -l 7") {
		t.Fatalf("ran %v", *ran)
	}
	ev := events(t, e.DB)
	if len(ev) != 1 || !strings.Contains(ev[0], `"app": "Finder"`) || !strings.Contains(ev[0], `"ok": true`) || !strings.Contains(ev[0], path) {
		t.Fatalf("events %v", ev)
	}
}

func TestGlobalAndPerAgentBlockRefuseAndNameTheSetting(t *testing.T) {
	e, ran := env(t, true, "block_screen_capture: true\n")
	_, err := shot(e, map[string]any{"agent": "a1", "app": "Finder"})
	if err == nil || !strings.Contains(err.Error(), "Block screen capture is on for every agent") {
		t.Fatalf("err %v", err)
	}
	if _, err := e.list(&rpc.Conn{}, map[string]any{"agent": "a1"}); err == nil {
		t.Fatal("list not refused")
	}
	for _, r := range *ran {
		if strings.HasPrefix(r, "screencapture") {
			t.Fatal("captured while blocked")
		}
	}
	if ev := events(t, e.DB); len(ev) != 1 || !strings.Contains(ev[0], `"status": "refused"`) {
		t.Fatalf("events %v", ev)
	}
	for _, q := range []string{`INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'w', '/x', 1)`, `INSERT INTO agents(id, workspace, name, owner, created_at, block_screen_capture) VALUES(1, 1, 'a', 'x', 1, 1)`} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(filepath.Join(e.Home, "config.yaml"))
	e2 := e
	_, err = shot(e2, map[string]any{"agent": "a1", "app": "Finder"})
	if err == nil || !strings.Contains(err.Error(), "for a1 (its agent settings)") {
		t.Fatalf("per agent: %v", err)
	}
	if _, err := shot(e2, map[string]any{"agent": "a2", "app": "Finder"}); err != nil {
		t.Fatalf("another agent: %v", err)
	}
}

func TestDeniedSaysWhereToAllowIt(t *testing.T) {
	e, _ := env(t, false, "")
	_, err := shot(e, map[string]any{"agent": "a1", "app": "Finder"})
	if err == nil || !strings.Contains(err.Error(), Denied) {
		t.Fatalf("err %v", err)
	}
	if ev := events(t, e.DB); len(ev) != 1 || !strings.Contains(ev[0], `"ok": false`) {
		t.Fatalf("events %v", ev)
	}
}

func TestPolicyFollowsTheSetting(t *testing.T) {
	off, _ := env(t, true, "")
	on, _ := env(t, true, "screencapture_via_handline: true\n")
	if off.Via() || !on.Via() {
		t.Fatal("via")
	}
}

func TestHistoryListsCapturesNewestFirstAndPictureReadsOnlyTheirFile(t *testing.T) {
	e, _ := env(t, true, "")
	if _, err := shot(e, map[string]any{"agent": "a1", "app": "Finder"}); err != nil {
		t.Fatal(err)
	}
	if _, err := shot(e, map[string]any{"agent": "a1", "app": "Nope"}); err == nil {
		t.Fatal("no window should fail")
	}
	got, err := e.history(&rpc.Conn{}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	cs := got.(map[string]any)["captures"].([]capture)
	if len(cs) != 2 || cs[0].Status != "failed" || cs[0].HasPicture || cs[1].Status != "ok" || !cs[1].HasPicture || cs[1].App != "Finder" || cs[1].Agent != "a1" {
		t.Fatalf("captures %+v", cs)
	}
	pic, err := e.picture(&rpc.Conn{}, map[string]any{"id": json.Number(fmt.Sprint(cs[1].ID))})
	if err != nil || pic.(map[string]any)["data"] != "cG5n" {
		t.Fatalf("picture %v %v", pic, err)
	}
	if _, err := e.picture(&rpc.Conn{}, map[string]any{"id": json.Number(fmt.Sprint(cs[0].ID))}); err == nil {
		t.Fatal("a failed capture has no picture")
	}
	outside := filepath.Join(t.TempDir(), "x.png")
	os.WriteFile(outside, []byte("secret"), 0o644)
	if _, err := e.DB.Exec(`UPDATE events SET payload = replace(payload, ?, ?) WHERE id = ?`, cs[1].Title, "", cs[1].ID); err != nil {
		t.Fatal(err)
	}
	var s string
	e.DB.QueryRow(`SELECT payload FROM events WHERE id = ?`, cs[1].ID).Scan(&s)
	var v map[string]any
	json.Unmarshal([]byte(s), &v)
	v["path"] = outside
	b, _ := json.Marshal(v)
	e.DB.Exec(`UPDATE events SET payload = ? WHERE id = ?`, string(b), cs[1].ID)
	if _, err := e.picture(&rpc.Conn{}, map[string]any{"id": json.Number(fmt.Sprint(cs[1].ID))}); err == nil {
		t.Fatal("a path outside the shots folder must not be read")
	}
}
