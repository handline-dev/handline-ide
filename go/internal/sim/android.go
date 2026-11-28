package sim

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type android struct{ e *Env }

const noAndroid = "Android: the Android SDK is not installed (no adb, or no emulator). Install it from the command line yourself: download the command-line tools from developer.android.com/studio, then `sdkmanager \"platform-tools\" \"emulator\" \"system-images;android-34;google_apis;arm64-v8a\"` and `avdmanager create avd -n handline -k \"system-images;android-34;google_apis;arm64-v8a\"`; set ANDROID_HOME to the SDK folder. Nothing in Handline installs it for you."

// tool is the SDK program name (adb, emulator): in the SDK folder ANDROID_HOME names, or the usual one, or on the PATH.
func (a *android) tool(sub, name string) (string, error) {
	var roots []string
	for _, v := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if r := os.Getenv(v); r != "" {
			roots = append(roots, r)
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(h, "Library", "Android", "sdk"), filepath.Join(h, "Android", "Sdk"))
	}
	for _, r := range roots {
		p := filepath.Join(r, sub, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", errors.New(noAndroid)
}

func (a *android) adb(serial string, args ...string) (string, error) {
	adb, err := a.tool("platform-tools", "adb")
	if err != nil {
		return "", err
	}
	if serial != "" {
		args = append([]string{"-s", serial}, args...)
	}
	return a.e.runOut(adb, args...)
}

func (a *android) list() ([]Device, error) {
	var ds []Device
	if adb, err := a.tool("platform-tools", "adb"); err == nil {
		out, _ := a.e.runOut(adb, "devices")
		for _, l := range strings.Split(out, "\n")[1:] {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "device" {
				name := f[0]
				if avd, err := a.e.runOut(adb, "-s", f[0], "emu", "avd", "name"); err == nil {
					name = strings.TrimSpace(strings.SplitN(avd, "\n", 2)[0])
				}
				ds = append(ds, Device{Platform: "android", ID: f[0], Name: name, Booted: true})
			}
		}
	} else {
		return nil, err
	}
	emulator, err := a.tool("emulator", "emulator")
	if err != nil {
		return ds, errors.New("Android: the emulator is not installed (`sdkmanager \"emulator\"`); devices adb already sees are listed")
	}
	out, _ := a.e.runOut(emulator, "-list-avds")
	for _, name := range strings.Split(out, "\n") {
		if name = strings.TrimSpace(name); name != "" && !strings.ContainsAny(name, " |") && !hasName(ds, name) {
			ds = append(ds, Device{Platform: "android", ID: name, Name: name})
		}
	}
	return ds, nil
}

func hasName(ds []Device, name string) bool {
	for _, d := range ds {
		if d.Name == name {
			return true
		}
	}
	return false
}

func (a *android) boot(d Device, agent string) (*Booted, error) {
	b := &Booted{Platform: "android", ID: d.ID, Name: d.Name, Agent: agent}
	if d.Booted {
		b.Adopted, b.Serial = true, d.ID
		return b, nil
	}
	emulator, err := a.tool("emulator", "emulator")
	if err != nil {
		return nil, err
	}
	port, err := freeConsolePort()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.e.dir("logs"), 0o755); err != nil {
		return nil, err
	}
	logf, err := os.Create(a.e.dir("logs", "emulator-"+d.ID+".log"))
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	cmd := exec.Command(emulator, "-avd", d.ID, "-port", strconv.Itoa(port), "-no-window", "-no-audio", "-no-boot-anim", "-no-snapshot-save")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	b.Serial = fmt.Sprintf("emulator-%d", port)
	ready := make(chan error, 1)
	go func() {
		if _, err := a.adb(b.Serial, "wait-for-device"); err != nil {
			ready <- err
			return
		}
		// The device's own shell waits for the boot to finish.
		_, err := a.adb(b.Serial, "shell", "while [ \"$(getprop sys.boot_completed)\" != 1 ]; do sleep 1; done")
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, err
		}
		return b, nil
	case <-exited:
		return nil, fmt.Errorf("the emulator %s ended while starting; its log is %s", d.Name, logf.Name())
	case <-time.After(10 * time.Minute):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("the emulator %s did not finish booting in 10 minutes; its log is %s", d.Name, logf.Name())
	}
}

// freeConsolePort is the first even port of the emulators' range whose console and adb ports are both free.
func freeConsolePort() (int, error) {
	for p := 5554; p <= 5584; p += 2 {
		free := true
		for _, q := range []int{p, p + 1} {
			l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", q))
			if err != nil {
				free = false
				break
			}
			l.Close()
		}
		if free {
			return p, nil
		}
	}
	return 0, errors.New("all of the emulator ports 5554 to 5585 are taken")
}

func (a *android) install(b *Booted, p string) error {
	_, err := a.adb(b.Serial, "install", "-r", p)
	return err
}

// launch starts app's launcher activity with am start: monkey, the usual way, exits 251 on an emulator with no hardware keys though it starts the app.
func (a *android) launch(b *Booted, app string) error {
	out, err := a.adb(b.Serial, "shell", "cmd", "package", "resolve-activity", "--brief", "-c", "android.intent.category.LAUNCHER", app)
	if err != nil {
		return err
	}
	lines := strings.Split(out, "\n")
	comp := strings.TrimSpace(lines[len(lines)-1])
	if !strings.Contains(comp, "/") {
		return fmt.Errorf("%s has no launcher activity: %s", app, out)
	}
	out, err = a.adb(b.Serial, "shell", "am", "start", "-W", "-n", comp)
	if err == nil && strings.Contains(out, "Error") {
		err = fmt.Errorf("am start %s: %s", comp, out)
	}
	return err
}

var boundsRE = regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

type xmlNode struct {
	Class    string    `xml:"class,attr"`
	Text     string    `xml:"text,attr"`
	Desc     string    `xml:"content-desc,attr"`
	ID       string    `xml:"resource-id,attr"`
	Bounds   string    `xml:"bounds,attr"`
	Enabled  string    `xml:"enabled,attr"`
	Selected string    `xml:"selected,attr"`
	Children []xmlNode `xml:"node"`
}

// ParseHierarchy is the elements of what `uiautomator dump` printed.
func ParseHierarchy(dump string) (*Node, error) {
	if i := strings.Index(dump, "</hierarchy>"); i >= 0 {
		dump = dump[:i+len("</hierarchy>")]
	}
	if i := strings.Index(dump, "<?xml"); i > 0 {
		dump = dump[i:]
	}
	var doc struct {
		Children []xmlNode `xml:"node"`
	}
	if err := xml.Unmarshal([]byte(dump), &doc); err != nil {
		return nil, fmt.Errorf("the screen could not be read: %w", err)
	}
	root := &Node{Type: "screen"}
	for _, c := range doc.Children {
		root.Children = append(root.Children, convert(c))
	}
	if len(root.Children) > 0 {
		root.Frame = root.Children[0].Frame
	}
	return root, nil
}

func convert(x xmlNode) *Node {
	n := &Node{Type: x.Class[strings.LastIndex(x.Class, ".")+1:], Label: x.Text, ID: x.ID, Selected: x.Selected == "true"}
	if x.Text == "" {
		n.Label = x.Desc
	}
	if x.Enabled == "false" {
		f := false
		n.Enabled = &f
	}
	if m := boundsRE.FindStringSubmatch(x.Bounds); m != nil {
		v := func(i int) float64 { f, _ := strconv.ParseFloat(m[i], 64); return f }
		n.Frame = Frame{v(1), v(2), v(3) - v(1), v(4) - v(2)}
	}
	for _, c := range x.Children {
		n.Children = append(n.Children, convert(c))
	}
	return n
}

func (a *android) tree(b *Booted) (*Node, error) {
	out, err := a.adb(b.Serial, "exec-out", "uiautomator", "dump", "/dev/tty")
	if err != nil {
		return nil, err
	}
	return ParseHierarchy(out)
}

func (a *android) tap(b *Booted, x, y float64) error {
	_, err := a.adb(b.Serial, "shell", "input", "tap", itoa(x), itoa(y))
	return err
}

func (a *android) swipe(b *Booted, x1, y1, x2, y2 float64) error {
	_, err := a.adb(b.Serial, "shell", "input", "swipe", itoa(x1), itoa(y1), itoa(x2), itoa(y2), "300")
	return err
}

func itoa(f float64) string { return strconv.Itoa(int(f + 0.5)) }

// inputText is text as `adb shell input text` takes it: its spaces as %s, the whole in single quotes for the device's shell.
func inputText(text string) string {
	t := strings.ReplaceAll(text, " ", "%s")
	return "'" + strings.ReplaceAll(t, "'", `'\''`) + "'"
}

func (a *android) typeText(b *Booted, text string) error {
	_, err := a.adb(b.Serial, "shell", "input text "+inputText(text))
	return err
}

var keys = map[string]string{"home": "3", "back": "4", "menu": "82", "volumeup": "24", "volumedown": "25", "power": "26", "enter": "66", "appswitch": "187"}

func (a *android) button(b *Booted, name string) error {
	code, ok := keys[name]
	if !ok {
		return fmt.Errorf("button is home, back, menu, volumeup, volumedown, power, enter or appswitch, not %q", name)
	}
	_, err := a.adb(b.Serial, "shell", "input", "keyevent", code)
	return err
}

func (a *android) rotate(b *Booted, to string) error {
	n, ok := map[string]string{"portrait": "0", "left": "1", "upsidedown": "2", "right": "3"}[to]
	if !ok {
		return fmt.Errorf("to is portrait, upsidedown, left or right, not %q", to)
	}
	// Android 14 no longer turns the display on a user_rotation setting alone; wm does, where it has user-rotation.
	// An old wm may print its error and still exit 0.
	if out, err := a.adb(b.Serial, "shell", "wm", "user-rotation", "lock", n); err == nil && !strings.Contains(out, "rror") && !strings.Contains(out, "nknown") {
		return nil
	}
	if _, err := a.adb(b.Serial, "shell", "settings", "put", "system", "accelerometer_rotation", "0"); err != nil {
		return err
	}
	_, err := a.adb(b.Serial, "shell", "settings", "put", "system", "user_rotation", n)
	return err
}

func (a *android) screenshot(b *Booted, file string) error {
	adb, err := a.tool("platform-tools", "adb")
	if err != nil {
		return err
	}
	out, err := a.e.runStdout(adb, "-s", b.Serial, "exec-out", "screencap", "-p")
	if err != nil {
		return fmt.Errorf("screencap: %v", err)
	}
	return os.WriteFile(file, out, 0o644)
}

func (a *android) shutdown(b *Booted) error {
	if b.Adopted {
		return nil
	}
	_, err := a.adb(b.Serial, "emu", "kill")
	return err
}
