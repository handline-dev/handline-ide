// Package screen serves screen.list, screen.shot and screen.policy: an agent captures one window through the daemon, never from its own
// shell, so macOS asks for screen recording once, for Handline, and each capture is kept as an event.
package screen

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/settings"
)

const (
	// BlockKey is the setting that refuses every capture; it is also an agent's own setting, over the global one.
	BlockKey = "block_screen_capture"
	// ViaKey makes the PreToolUse hook refuse a shell screencapture and name the way through Handline.
	ViaKey = "screencapture_via_handline"
	// Event is the kind of the event each capture leaves.
	Event = "screen.capture"
)

// Denied is what an agent reads when macOS refuses the capture to Handline.
const Denied = "macOS denied screen recording for Handline: System Settings > Privacy & Security > Screen & System Audio Recording, turn Handline on"

// Window is one on-screen window.
type Window struct {
	ID    int64  `json:"id"`
	App   string `json:"app"`
	Title string `json:"title"`
}

// Env is what the methods need. Run runs a command and answers its combined output (exec by default).
type Env struct {
	DB   *sql.DB
	Home string
	Run  func(name string, args ...string) ([]byte, error)
	Now  func() time.Time
}

// listScript lists the on-screen windows front to back (CoreGraphics through JXA, which every Mac has) and says whether screen recording
// is granted, in one JSON object.
const listScript = `ObjC.import('CoreGraphics');
var l = ObjC.deepUnwrap(ObjC.castRefToObject($.CGWindowListCopyWindowInfo(1|16, 0)));
JSON.stringify({access: $.CGPreflightScreenCaptureAccess(), windows: l.filter(function(w){return w.kCGWindowLayer===0}).map(function(w){
return {id: w.kCGWindowNumber, app: w.kCGWindowOwnerName, title: w.kCGWindowName||''}})})`

func (e *Env) run(name string, args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(name, args...)
	}
	cmd := exec.Command(name, args...)
	var errs bytes.Buffer
	cmd.Stderr = &errs
	out, err := cmd.Output()
	if err != nil && errs.Len() > 0 {
		out = append(out, errs.Bytes()...)
	}
	return out, err
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Windows is the on-screen windows, front to back, and whether macOS lets Handline record the screen.
func (e *Env) Windows() ([]Window, bool, error) {
	out, err := e.run("osascript", "-l", "JavaScript", "-e", listScript)
	if err != nil {
		return nil, false, fmt.Errorf("could not list the windows: %v", err)
	}
	var got struct {
		Access  bool     `json:"access"`
		Windows []Window `json:"windows"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &got); err != nil {
		return nil, false, fmt.Errorf("could not read the window list: %v", err)
	}
	return got.Windows, got.Access, nil
}

// Blocked is why agent may not capture: the setting that says so, or "".
func (e *Env) Blocked(agent string) string {
	if (&settings.Settings{Home: e.Home}).Flag(BlockKey, false) {
		return "Block screen capture is on for every agent (Settings)"
	}
	if cfg, err := team.NewRoster(e.Home).Read(agent); err == nil {
		if b, _ := cfg[BlockKey].(bool); b {
			return "Block screen capture is on for " + agent + " (its agent settings)"
		}
	}
	return ""
}

// Via is whether the shell screencapture is to be refused (the setting Send screenshot commands through Handline).
func (e *Env) Via() bool { return (&settings.Settings{Home: e.Home}).Flag(ViaKey, false) }

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (e *Env) own(c *rpc.Conn, p map[string]any) (string, error) {
	agent, _ := p["agent"].(string)
	if agent == "" {
		return "", rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
	}
	return agent, c.RequireSelf(agent)
}

// Register adds the methods.
func (e *Env) Register(s *rpc.Server) {
	s.Register("screen.policy", rpc.Method{Read: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		return map[string]any{"via": e.Via()}, nil
	}})
	s.Register("screen.list", rpc.Method{Read: true, Handler: e.list})
	s.Register("screen.shot", rpc.Method{Read: true, Handler: e.shot})
}

func (e *Env) list(c *rpc.Conn, p map[string]any) (any, error) {
	agent, err := e.own(c, p)
	if err != nil {
		return nil, err
	}
	if why := e.Blocked(agent); why != "" {
		return nil, rpc.Refused("hl screen is refused: "+why, "", "")
	}
	ws, _, err := e.Windows()
	if err != nil {
		return nil, err
	}
	return map[string]any{"windows": ws}, nil
}

func (e *Env) shot(c *rpc.Conn, p map[string]any) (any, error) {
	agent, err := e.own(c, p)
	if err != nil {
		return nil, err
	}
	app, _ := p["app"].(string)
	want, _ := rpc.AsInt(p["window"])
	if app == "" && want == 0 {
		return nil, rpc.BadRequestError("name an app or a window", "app", rpc.InvalidParams)
	}
	task := int64(0)
	if t, held, err := db.CurrentTask(e.DB, agent); err == nil && held {
		task = t
	}
	note := func(w Window, path, status, why string) {
		kv := []db.KV{{K: "agent", V: agent}, {K: "task", V: task}, {K: "app", V: w.App}, {K: "title", V: w.Title}, {K: "path", V: path}, {K: "ok", V: status == "ok"}, {K: "status", V: status}}
		if why != "" {
			kv = append(kv, db.KV{K: "reason", V: why})
		}
		db.AddEvent(e.DB, agent, Event, kv...)
	}
	if why := e.Blocked(agent); why != "" {
		note(Window{App: app}, "", "refused", why)
		return nil, rpc.Refused("hl screen is refused: "+why, "", "")
	}
	ws, _, err := e.Windows()
	if err != nil {
		return nil, err
	}
	var w *Window
	for i := range ws {
		if (want != 0 && ws[i].ID == want) || (want == 0 && strings.EqualFold(ws[i].App, app)) {
			w = &ws[i]
			break
		}
	}
	if w == nil {
		what := app
		if want != 0 {
			what = fmt.Sprintf("window %d", want)
		}
		note(Window{App: app}, "", "failed", "no on-screen window for "+what)
		return nil, rpc.NotFoundError("no on-screen window for " + what)
	}
	dir := filepath.Join(e.Home, "shots", agent)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	stem := filepath.Join(dir, fmt.Sprintf("%s-%s", e.now().Format("20060102-150405"), strings.Trim(unsafeName.ReplaceAllString(w.App, "-"), "-")))
	path := stem + ".png"
	for n := 2; ; n++ {
		if _, err := os.Stat(path); err != nil {
			break
		}
		path = fmt.Sprintf("%s-%d.png", stem, n)
	}
	out, runErr := e.run("screencapture", "-l", fmt.Sprint(w.ID), "-x", path)
	_, statErr := os.Stat(path)
	_, access, accErr := e.Windows()
	if accErr != nil { // access could not be read: judge by the capture itself
		access = runErr == nil && statErr == nil
	}
	if !access || runErr != nil || statErr != nil {
		os.Remove(path)
		why := strings.TrimSpace(string(out))
		if !access {
			why = Denied
		}
		if why == "" {
			why = "the capture failed"
		}
		note(*w, "", "failed", why)
		return nil, rpc.Refused(why, "", "")
	}
	note(*w, path, "ok", "")
	return map[string]any{"path": path, "app": w.App, "title": w.Title, "window": w.ID}, nil
}
