package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheTreeIsListedWhereItSaysSomethingAndTheFirstEnabledMatchIsFound(t *testing.T) {
	off := false
	root := &Node{Type: "application", Label: "Settings", Frame: Frame{0, 0, 390, 844}, Children: []*Node{
		{Type: "other", Frame: Frame{0, 0, 390, 844}, Children: []*Node{
			{Type: "button", Label: "General", ID: "general", Frame: Frame{16, 278, 358, 52}, Enabled: &off},
			{Type: "button", Label: "General", ID: "general2", Frame: Frame{16, 330, 358, 52}},
			{Type: "image", Frame: Frame{0, 0, 5, 5}},
		}},
	}}
	var out bytes.Buffer
	Render(&out, root)
	want := "application \"Settings\" @0,0 390x844\n  button \"General\" #general disabled @16,278 358x52\n  button \"General\" #general2 @16,330 358x52\n"
	if out.String() != want {
		t.Errorf("Render =\n%s\nwant\n%s", out.String(), want)
	}
	n, ok := Find(root, Selector{Label: "General"})
	if !ok || n.ID != "general2" {
		t.Errorf("Find label General = %v, %v: a disabled element is not tapped", n, ok)
	}
	if x, y := n.Frame.Center(); x != 195 || y != 356 {
		t.Errorf("center = %v,%v", x, y)
	}
	if n, ok := Find(root, Selector{Text: "GENERAL2"}); !ok || n.ID != "general2" {
		t.Errorf("Find text GENERAL2 = %v, %v", n, ok)
	}
	if _, ok := Find(root, Selector{}); ok {
		t.Error("an empty selector found something")
	}
}

func TestADriverTreeReadsBackAsTheDriverWritesIt(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(`{"type":"button","label":"OK","frame":[1,2,3,4],"enabled":false,"children":[{"type":"image","frame":[0,0,1,1]}]}`), &n); err != nil {
		t.Fatal(err)
	}
	if n.Label != "OK" || !n.disabled() || n.Frame != (Frame{1, 2, 3, 4}) || len(n.Children) != 1 {
		t.Errorf("node = %+v", n)
	}
}

func TestAndroidsHierarchyBecomesTheSameTree(t *testing.T) {
	dump := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0"><node class="android.widget.FrameLayout" text="" content-desc="" resource-id="" bounds="[0,0][1080,2400]" enabled="true" selected="false">
<node class="android.widget.TextView" text="Network" content-desc="" resource-id="android:id/title" bounds="[210,721][772,812]" enabled="true" selected="false"/>
<node class="android.widget.ImageButton" text="" content-desc="Navigate up" resource-id="" bounds="[0,132][147,279]" enabled="false" selected="false"/>
</node></hierarchy>
UI hierchary dumped to: /dev/tty`
	root, err := ParseHierarchy(dump)
	if err != nil {
		t.Fatal(err)
	}
	if root.Frame != (Frame{0, 0, 1080, 2400}) {
		t.Errorf("screen frame = %v", root.Frame)
	}
	n, ok := Find(root, Selector{Label: "Network"})
	if !ok || n.Type != "TextView" || n.ID != "android:id/title" || n.Frame != (Frame{210, 721, 562, 91}) {
		t.Errorf("Network = %+v, %v", n, ok)
	}
	if _, ok := Find(root, Selector{Label: "Navigate up"}); ok {
		t.Error("a disabled element was found for a tap")
	}
}

func TestTextForAdbIsQuotedForTheDevicesShell(t *testing.T) {
	if got, want := inputText("it's 50% off"), `'it'\''s%s50%%soff'`; got != want {
		t.Errorf("inputText = %s, want %s", got, want)
	}
}

func TestADeviceIsPickedByNameOrAsTheAgentsOnlyOne(t *testing.T) {
	e := &Env{Home: t.TempDir()}
	for _, b := range []*Booted{{Platform: "ios", ID: "U1", Name: "phone", Agent: "a"}, {Platform: "android", ID: "pixel", Name: "pixel", Agent: "b"}} {
		if err := e.save(b); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := e.Pick("a", ""); err != nil || b.ID != "U1" {
		t.Errorf("a's only device = %v, %v", b, err)
	}
	if b, err := e.Pick("a", "pixel"); err != nil || b.ID != "pixel" {
		t.Errorf("named = %v, %v", b, err)
	}
	if _, err := e.Pick("c", ""); err == nil || !strings.Contains(err.Error(), "hl sim boot") {
		t.Errorf("no device for c: %v", err)
	}
	if err := e.save(&Booted{Platform: "ios", ID: "U2", Name: "tablet", Agent: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Pick("a", ""); err == nil || !strings.Contains(err.Error(), "--device") {
		t.Errorf("two devices: %v", err)
	}
}

func TestASessionsEndShutsDownWhatItBootedAndLeavesWhatItFound(t *testing.T) {
	var calls []string
	e := &Env{Home: t.TempDir(), Run: func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}}
	for _, b := range []*Booted{
		{Platform: "android", ID: "pixel", Name: "pixel", Agent: "a", Serial: "emulator-5554"},
		{Platform: "android", ID: "emulator-5556", Name: "mine", Agent: "a", Serial: "emulator-5556", Adopted: true},
		{Platform: "android", ID: "other", Name: "other", Agent: "b", Serial: "emulator-5558"},
	} {
		if err := e.save(b); err != nil {
			t.Fatal(err)
		}
	}
	// The SDK's adb must be found, or the shutdown fails and the device stays: a machine with no Android SDK would fail this test otherwise.
	sdk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sdk, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "platform-tools", "adb"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_HOME", sdk)
	e.Reap("a")
	if left := e.All(); len(left) != 1 || left[0].Agent != "b" {
		t.Errorf("left after a's session = %v", left)
	}
	for _, c := range calls {
		if strings.Contains(c, "emulator-5556") || strings.Contains(c, "emulator-5558") {
			t.Errorf("a device a did not boot was touched: %s", c)
		}
	}
}

func TestAndroidLaunchStartsTheLauncherActivityAndRotateLocksTheDisplayWithWm(t *testing.T) {
	sdk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sdk, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "platform-tools", "adb"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_HOME", sdk)
	var calls []string
	noWm := false
	e := &Env{Home: t.TempDir(), Run: func(name string, args ...string) ([]byte, error) {
		c := strings.Join(args, " ")
		calls = append(calls, c)
		switch {
		case strings.Contains(c, "resolve-activity"):
			return []byte("priority=0 preferredOrder=0\ncom.android.settings/.Settings\n"), nil
		case strings.Contains(c, "wm user-rotation") && noWm:
			return []byte("Unknown command"), errors.New("exit status 255")
		}
		return []byte("Status: ok"), nil
	}}
	a, b := &android{e: e}, &Booted{Serial: "emulator-5554"}
	if err := a.launch(b, "com.android.settings"); err != nil {
		t.Fatal(err)
	}
	if want := "-s emulator-5554 shell am start -W -n com.android.settings/.Settings"; calls[len(calls)-1] != want {
		t.Errorf("launch ran %q, want %q", calls, want)
	}
	calls = nil
	if err := a.rotate(b, "left"); err != nil || len(calls) != 1 || calls[0] != "-s emulator-5554 shell wm user-rotation lock 1" {
		t.Errorf("rotate left ran %q, %v", calls, err)
	}
	calls, noWm = nil, true
	if err := a.rotate(b, "right"); err != nil || len(calls) != 3 || calls[2] != "-s emulator-5554 shell settings put system user_rotation 3" {
		t.Errorf("rotate right with no wm user-rotation ran %q, %v", calls, err)
	}
}
