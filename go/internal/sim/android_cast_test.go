package sim

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// panel is a 2x4 emulator panel: red at its top left, the rest blue.
func panel() *image.NRGBA {
	p := image.NewNRGBA(image.Rect(0, 0, 2, 4))
	for y := range 4 {
		for x := range 2 {
			p.Set(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	p.Set(0, 0, color.NRGBA{R: 255, A: 255})
	return p
}

func TestAPanelIsTurnedTheWayTheDisplayIs(t *testing.T) {
	// Where the panel's top left lands once turned: a display turned a quarter shows the panel's left edge at its bottom.
	for turn, want := range [][2]int{{0, 0}, {0, 1}, {1, 3}, {3, 0}} {
		img, w, h := turnAndShrink(panel(), turn, 800)
		if turn%2 == 1 && (w != 4 || h != 2) || turn%2 == 0 && (w != 2 || h != 4) {
			t.Errorf("turn %d: size %dx%d", turn, w, h)
		}
		if r, _, _, _ := img.At(want[0], want[1]).RGBA(); r != 0xffff {
			t.Errorf("turn %d: the panel's top left is not at %v", turn, want)
		}
	}
	if img, w, h := turnAndShrink(image.NewNRGBA(image.Rect(0, 0, 1080, 2400)), 1, 800); img.Rect.Dx() != 800 || img.Rect.Dy() != 360 || w != 2400 || h != 1080 {
		t.Errorf("shrunk to %v, size %dx%d: the picture at most 800 long, the size the display's", img.Rect, w, h)
	}
}

func TestAnEmulatorFrameIsItsConsolePictureTurnedAndAPhoneHasNone(t *testing.T) {
	sdk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sdk, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "platform-tools", "adb"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_HOME", sdk)
	rotation := "      mCurrentRotation=ROTATION_270\n"
	var pic bytes.Buffer
	if err := png.Encode(&pic, panel()); err != nil {
		t.Fatal(err)
	}
	e := &Env{Run: func(name string, args ...string) ([]byte, error) {
		switch {
		case slices.Equal(args[:len(args)-1], []string{"-s", "emulator-5554", "emu", "screenrecord", "screenshot"}):
			return []byte("OK"), os.WriteFile(filepath.Join(args[len(args)-1], "Screenshot_1.png"), pic.Bytes(), 0o644)
		case slices.Equal(args, []string{"-s", "emulator-5554", "shell", "dumpsys", "window", "displays"}):
			return []byte("  mDisplayId=0\n" + rotation), nil
		}
		t.Errorf("adb %v", args)
		return nil, nil
	}}
	a := &android{e: e}
	f, err := a.frame(&Booted{Platform: "android", Serial: "emulator-5554"}, 800, 0.7)
	if err != nil {
		t.Fatal(err)
	}
	if f.Width != 4 || f.Height != 2 {
		t.Errorf("frame size %dx%d, want the turned display in pixels", f.Width, f.Height)
	}
	if img, err := jpeg.Decode(bytes.NewReader(f.JPEG)); err != nil || img.Bounds().Dx() != 4 {
		t.Errorf("frame %v, %v: a 4x2 JPEG", img, err)
	}
	rotation = ""
	if _, err := a.frame(&Booted{Platform: "android", Serial: "emulator-5554"}, 800, 0.7); err == nil {
		t.Error("a frame was sent with the display's rotation unknown: it could be sideways, and taps land wrong")
	}
	if _, err := a.frame(&Booted{Platform: "android", Serial: "R58M123"}, 800, 0.7); err == nil {
		t.Error("a phone on adb was pictured: only an emulator has the console")
	}
}
