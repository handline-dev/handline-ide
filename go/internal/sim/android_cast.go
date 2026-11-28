package sim

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/handline/handline/go/internal/screenpipe"
)

var rotationRE = regexp.MustCompile(`mCurrentRotation=ROTATION_(0|90|180|270)`)

// pictures is whether the device has an emulator's console; a phone on adb has none.
func (a *android) pictures(b *Booted) bool { return strings.HasPrefix(b.Serial, "emulator-") }

// rotation is how many quarters anticlockwise the display is turned.
func (a *android) rotation(b *Booted) (int, error) {
	out, err := a.adb(b.Serial, "shell", "dumpsys", "window", "displays")
	if err != nil {
		return 0, err
	}
	m := rotationRE.FindStringSubmatch(out)
	if m == nil {
		return 0, errors.New("the display's rotation was not in dumpsys")
	}
	return map[string]int{"0": 0, "90": 1, "180": 2, "270": 3}[m[1]], nil
}

// frame is the emulator's own picture of its panel, taken through its console (about 90 ms, where adb's screencap takes 360 to 500),
// turned the way the display is and made at most most pixels on its longest side. Its size is the display's in pixels, as adb input
// takes them.
func (a *android) frame(b *Booted, most int, quality float64) (screenpipe.Frame, error) {
	if !a.pictures(b) {
		return screenpipe.Frame{}, errors.New("only an emulator can be pictured, not a device on adb")
	}
	dir, err := os.MkdirTemp("", "hl-frame-")
	if err != nil {
		return screenpipe.Frame{}, fmt.Errorf("emulator frame: %w", err)
	}
	defer os.RemoveAll(dir)
	type turned struct {
		n   int
		err error
	}
	turns := make(chan turned, 1)
	go func() {
		n, err := a.rotation(b)
		turns <- turned{n, err}
	}()
	said, err := a.adb(b.Serial, "emu", "screenrecord", "screenshot", dir)
	turn := <-turns
	if err != nil {
		return screenpipe.Frame{}, err
	}
	if turn.err != nil {
		return screenpipe.Frame{}, fmt.Errorf("emulator frame: %w", turn.err)
	}
	shots, _ := filepath.Glob(filepath.Join(dir, "*.png")) // the pattern is fixed, so it never errs
	if len(shots) != 1 {
		return screenpipe.Frame{}, fmt.Errorf("the emulator wrote no picture: %s", said)
	}
	raw, err := os.ReadFile(shots[0])
	if err != nil {
		return screenpipe.Frame{}, fmt.Errorf("emulator frame: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return screenpipe.Frame{}, fmt.Errorf("emulator frame: %w", err)
	}
	small, w, h := turnAndShrink(img, turn.n, most)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, small, &jpeg.Options{Quality: int(quality * 100)}); err != nil {
		return screenpipe.Frame{}, fmt.Errorf("emulator frame: %w", err)
	}
	return screenpipe.Frame{JPEG: out.Bytes(), Width: w, Height: h}, nil
}

// turnAndShrink is the panel turned turn quarters anticlockwise and shrunk so its longest side is at most most pixels, and the turned size.
func turnAndShrink(img image.Image, turn, most int) (*image.RGBA, int, int) {
	src, ok := img.(*image.NRGBA)
	if !ok {
		src = image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
		draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	}
	pw, ph := src.Rect.Dx(), src.Rect.Dy()
	w, h := pw, ph
	if turn%2 == 1 {
		w, h = ph, pw
	}
	ow, oh := w, h
	if long := max(w, h); long > most {
		ow, oh = (w*most+long/2)/long, (h*most+long/2)/long
	}
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for oy := range oh {
		ty := oy * h / oh
		for ox := range ow {
			tx := ox * w / ow
			x, y := tx, ty
			switch turn {
			case 1:
				x, y = pw-1-ty, tx
			case 2:
				x, y = pw-1-tx, ph-1-ty
			case 3:
				x, y = ty, ph-1-tx
			}
			s := src.Pix[y*src.Stride+4*x:]
			d := dst.Pix[oy*dst.Stride+4*ox:]
			d[0], d[1], d[2], d[3] = s[0], s[1], s[2], 255
		}
	}
	return dst, w, h
}
