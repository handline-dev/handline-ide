package browser

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A page's size is DevTools' device metrics override, which lasts only while the session that set it is open, and a window does not get
// narrower than about 500 pixels. So a process of its own holds that session for the page until the page or the browser closes, or the
// page is sized again.

// Keep sizes page id to width by height, calls ready, and holds the size while the page lives.
func (b *Browser) Keep(id string, width, height int, ready func()) error {
	list, err := b.pages()
	if err != nil {
		return err
	}
	ws := ""
	for _, t := range list {
		if t.ID == id {
			ws = t.WS
		}
	}
	if ws == "" {
		return errors.New("the tab has closed")
	}
	c, err := dial(ws)
	if err != nil {
		return err
	}
	defer c.close()
	if _, err := c.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 0,
		"mobile": false}); err != nil {
		return err
	}
	ready()
	_ = c.ws.SetReadDeadline(time.Time{})
	for {
		if _, _, err := c.ws.ReadMessage(); err != nil {
			return nil
		}
	}
}

func keeperFile(dir, id string) string { return filepath.Join(dir, "size-"+id+".pid") }

// keptWithin is how long a process holding a size has to say it holds it.
const keptWithin = 10 * time.Second

// Resize sizes the page to width by height, ending what held its last size and running keeper with the page's id, width and height to
// hold the new one; keeper prints a line once it holds it.
func (p *Page) Resize(width, height int, keeper ...string) error {
	return held(p.b.Dir, func() error { return p.resize(width, height, keeper) })
}

func (p *Page) resize(width, height int, keeper []string) error {
	id, dir := p.b.PageID, p.b.Dir
	if b, err := os.ReadFile(keeperFile(dir, id)); err == nil {
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 0 && keeping(pid, id) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			// its session's end clears the size, so the new one is set only once it has gone
			for end := time.Now().Add(stopWithin); keeping(pid, id) && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			}
		}
	}
	log, err := os.Create(filepath.Join(dir, "size.log"))
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(keeper[0], append(keeper[1:], id, strconv.Itoa(width), strconv.Itoa(height))...)
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // outlives this hl, as the browser does
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	said := make(chan bool, 1)
	go func() { said <- bufio.NewScanner(out).Scan() }()
	select {
	case ok := <-said:
		if !ok {
			cmd.Wait()
			return fmt.Errorf("the page could not be sized; see %s", log.Name())
		}
	case <-time.After(keptWithin):
		_ = cmd.Process.Kill()
		return fmt.Errorf("the page was not sized within %s; see %s", keptWithin, log.Name())
	}
	go cmd.Wait()
	return os.WriteFile(keeperFile(dir, id), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
}

// keeping is whether pid is still the process holding page id's size: a pid can be reused.
func keeping(pid int, id string) bool {
	out, err := exec.Command("ps", "-ww", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.Contains(string(out), " "+id+" ")
}
