package sim

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed driver
var driverFiles embed.FS

type ios struct{ e *Env }

const noXcode = "iOS: Xcode is not installed or not selected (xcrun simctl is missing). Install Xcode from the App Store, open it once to accept the licence, and run `xcode-select -s /Applications/Xcode.app`; then hl sim works. Install it yourself, and ask the founder only for the Apple login Xcode wants."

func (i *ios) list() ([]Device, error) {
	out, err := i.e.runOut("xcrun", "simctl", "list", "devices", "available", "-j")
	if err != nil {
		return nil, errors.New(noXcode)
	}
	var doc struct {
		Devices map[string][]struct{ UDID, Name, State string } `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil, err
	}
	var ds []Device
	for runtime, list := range doc.Devices {
		if !strings.Contains(runtime, "iOS") {
			continue
		}
		for _, d := range list {
			ds = append(ds, Device{Platform: "ios", ID: d.UDID, Name: d.Name, Booted: d.State == "Booted"})
		}
	}
	return ds, nil
}

func (i *ios) boot(d Device, agent string) (b *Booted, err error) {
	b = &Booted{Platform: "ios", ID: d.ID, Name: d.Name, Agent: agent, Adopted: d.Booted}
	var serve *exec.Cmd
	defer func() {
		if err == nil {
			return
		}
		if serve != nil {
			_ = serve.Process.Kill()
		}
		if !b.Adopted {
			_, _ = i.e.runOut("xcrun", "simctl", "shutdown", d.ID)
		}
	}()
	if _, err = i.e.runOut("xcrun", "simctl", "bootstatus", d.ID, "-b"); err != nil {
		return nil, err
	}
	xctestrun, err := i.driver()
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(i.e.dir("logs"), 0o755); err != nil {
		return nil, err
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer pr.Close()
	serve = exec.Command(self, "sim", "_serve", d.ID, xctestrun)
	serve.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	serve.Env = append(os.Environ(), "HANDLINE_HOME="+i.e.Home)
	serve.Stdout = pw
	err = serve.Start()
	pw.Close() // the serve process holds the only write end: it ending is the end of the read
	if err != nil {
		serve = nil
		return nil, err
	}
	go func() { _ = serve.Wait() }()
	_ = pr.SetReadDeadline(time.Now().Add(10 * time.Minute))
	line, _ := bufio.NewReader(pr).ReadString('\n')
	port, perr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ready")))
	if perr != nil || !strings.HasPrefix(line, "ready") {
		return nil, fmt.Errorf("the driver did not start on %s; its log is %s", d.Name, i.e.dir("logs", d.ID+".log"))
	}
	b.Port, b.Serve = port, serve.Process.Pid
	return b, nil
}

// driver is the built UI test bundle's .xctestrun, built from driver/ first when this Xcode has not built it yet.
func (i *ios) driver() (string, error) {
	version, err := i.e.runOut("xcodebuild", "-version")
	if err != nil {
		return "", errors.New(noXcode)
	}
	h := sha256.New()
	h.Write([]byte(version))
	_ = fs.WalkDir(driverFiles, "driver", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := driverFiles.ReadFile(p)
			h.Write([]byte(p + "\x00"))
			h.Write(b)
		}
		return nil
	})
	root := i.e.dir("driver", hex.EncodeToString(h.Sum(nil))[:12])
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil { // another agent's first boot builds it once
		return "", err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	built := func() string {
		m, _ := filepath.Glob(filepath.Join(root, "dd", "Build", "Products", "*.xctestrun"))
		if len(m) > 0 {
			if _, err := os.Stat(filepath.Join(root, "built")); err == nil {
				return m[0]
			}
		}
		return ""
	}
	if f := built(); f != "" {
		return f, nil
	}
	src := filepath.Join(root, "src")
	_ = os.RemoveAll(src)
	err = fs.WalkDir(driverFiles, "driver", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := driverFiles.ReadFile(p)
		out := filepath.Join(src, filepath.FromSlash(strings.TrimPrefix(p, "driver/")))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		return "", err
	}
	if _, err := i.e.runOut("xcodebuild", "build-for-testing", "-project", filepath.Join(src, "HandlineSim.xcodeproj"), "-scheme", "HandlineSim",
		"-destination", "generic/platform=iOS Simulator", "-derivedDataPath", filepath.Join(root, "dd"), "-quiet"); err != nil {
		return "", fmt.Errorf("building the simulator driver failed (Xcode is installed but could not build it): %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "built"), nil, 0o644); err != nil {
		return "", err
	}
	if f := built(); f != "" {
		return f, nil
	}
	return "", errors.New("the simulator driver was built but left no .xctestrun")
}

// Serve is the driver's own process (hl sim _serve): it runs the UI test on the device, says "ready <port>" on its output once the test
// listens, and ends with the test.
func Serve(home, udid, xctestrun string, say io.Writer) error {
	e := &Env{Home: home}
	if err := os.MkdirAll(e.dir("logs"), 0o755); err != nil {
		return err
	}
	logf, err := os.Create(e.dir("logs", udid+".log"))
	if err != nil {
		return err
	}
	defer logf.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	cmd := exec.CommandContext(ctx, "xcodebuild", "test-without-building", "-xctestrun", xctestrun, "-destination", "id="+udid)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	told := false
	for sc.Scan() {
		fmt.Fprintln(logf, sc.Text())
		if _, rest, ok := strings.Cut(sc.Text(), "HANDLINE-SIM-READY "); ok && !told {
			told = true
			fmt.Fprintln(say, "ready "+strings.TrimSpace(rest))
		}
	}
	return cmd.Wait()
}

// call is one request to b's driver.
func (i *ios) call(b *Booted, req map[string]any) (map[string]any, error) {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", b.Port), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("the driver of %s is not answering (hl sim shutdown, then boot again): %w", b.Name, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(90 * time.Second))
	if _, ok := req["bundle"]; !ok && b.App != "" {
		req["bundle"] = b.App
	}
	data, _ := json.Marshal(req)
	if _, err := c.Write(append(data, '\n')); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var got map[string]any
	if err := json.Unmarshal(line, &got); err != nil {
		return nil, err
	}
	if ok, _ := got["ok"].(bool); !ok {
		why, _ := got["error"].(string)
		return nil, errors.New(why)
	}
	return got, nil
}

func (i *ios) install(b *Booted, p string) error {
	_, err := i.e.runOut("xcrun", "simctl", "install", b.ID, p)
	return err
}

func (i *ios) launch(b *Booted, app string) error {
	_, err := i.e.runOut("xcrun", "simctl", "launch", b.ID, app)
	return err
}

func (i *ios) tree(b *Booted) (*Node, error) {
	got, err := i.call(b, map[string]any{"op": "tree"})
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(got["tree"])
	var n Node
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (i *ios) tap(b *Booted, x, y float64) error {
	_, err := i.call(b, map[string]any{"op": "tap", "x": x, "y": y})
	return err
}

func (i *ios) swipe(b *Booted, x1, y1, x2, y2 float64) error {
	_, err := i.call(b, map[string]any{"op": "swipe", "x1": x1, "y1": y1, "x2": x2, "y2": y2})
	return err
}

func (i *ios) typeText(b *Booted, text string) error {
	_, err := i.call(b, map[string]any{"op": "type", "text": text})
	return err
}

func (i *ios) button(b *Booted, name string) error {
	_, err := i.call(b, map[string]any{"op": "button", "name": name})
	return err
}

func (i *ios) rotate(b *Booted, to string) error {
	_, err := i.call(b, map[string]any{"op": "rotate", "to": to})
	return err
}

func (i *ios) screenshot(b *Booted, file string) error {
	_, err := i.e.runOut("xcrun", "simctl", "io", b.ID, "screenshot", "--type=png", file)
	return err
}

func (i *ios) shutdown(b *Booted) error {
	_, _ = i.call(b, map[string]any{"op": "quit"})
	if b.Serve > 0 {
		_ = syscall.Kill(b.Serve, syscall.SIGTERM) // its xcodebuild would wait on a device that is going away
	}
	if b.Adopted {
		return nil
	}
	if _, err := i.e.runOut("xcrun", "simctl", "shutdown", b.ID); err != nil && !strings.Contains(err.Error(), "current state: Shutdown") {
		return err
	}
	return nil
}
