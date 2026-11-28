package sim

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/screenpipe"
)

// driver stands in for the iOS driver: one JSON request per connection, frames from pics in turn (the last repeats), every other op kept.
type driver struct {
	mu   sync.Mutex
	l    net.Listener
	pics []string
	n    int
	ops  []map[string]any
}

func newDriver(t *testing.T, pics ...string) *driver {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := &driver{l: l, pics: pics}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go d.serve(c)
		}
	}()
	return d
}

func (d *driver) serve(c net.Conn) {
	defer c.Close()
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return
	}
	var req map[string]any
	_ = json.Unmarshal(line, &req)
	out := map[string]any{"ok": true}
	d.mu.Lock()
	if req["op"] == "frame" {
		pic := d.pics[min(d.n, len(d.pics)-1)]
		d.n++
		out["jpeg"], out["width"], out["height"] = base64.StdEncoding.EncodeToString([]byte(pic)), 420.0, 912.0
	} else {
		d.ops = append(d.ops, req)
	}
	d.mu.Unlock()
	b, _ := json.Marshal(out)
	_, _ = c.Write(append(b, '\n'))
}

func (d *driver) port() int { return d.l.Addr().(*net.TCPAddr).Port }

func (d *driver) got() []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]map[string]any{}, d.ops...)
}

func (d *driver) frames() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.n
}

func castOn(t *testing.T, d *driver) *Cast {
	e := &Env{Home: t.TempDir()}
	if err := e.save(&Booted{Platform: "ios", ID: "U1", Name: "iPhone", Agent: "a1", Port: d.port(), Since: 1}); err != nil {
		t.Fatal(err)
	}
	s, err := Opener(e.Home)("a1", Source)
	if err != nil {
		t.Fatal(err)
	}
	c := s.(*Cast)
	c.Every, c.Retry = time.Millisecond, time.Millisecond
	return c
}

func TestAnAgentsDeviceIsItsSourceAndNoOneElsesIs(t *testing.T) {
	e := &Env{Home: t.TempDir()}
	_ = e.save(&Booted{Platform: "ios", ID: "U1", Agent: "a1", Since: 1})
	_ = e.save(&Booted{Platform: "ios", ID: "U2", Agent: "a1", Since: 5})
	_ = e.save(&Booted{Platform: "android", ID: "emu", Agent: "a2", Since: 9, Serial: "emulator-5554"})
	_ = e.save(&Booted{Platform: "android", ID: "phone", Agent: "a2", Since: 10, Serial: "R58M123", Adopted: true})
	open := Opener(e.Home)
	s, err := open("a1", Source)
	if err != nil || s.(*Cast).b.ID != "U2" {
		t.Fatalf("a1's source = %v, %v: the device it booted last", s, err)
	}
	if s, err := open("a2", Source); err != nil || s.(*Cast).b.ID != "emu" {
		t.Errorf("a2's source = %v, %v: its emulator", s, err)
	}
	for _, c := range [][2]string{{"a3", Source}, {"a1", "browser"}} {
		if _, err := open(c[0], c[1]); !errors.Is(err, screenpipe.ErrNotOpen) {
			t.Errorf("%s's %s = %v, want not open", c[0], c[1], err)
		}
	}
}

func TestADeviceSendsAFrameOnlyWhenItChangesAndStopsAskingWhenStopped(t *testing.T) {
	d := newDriver(t, "a", "a", "b", "b", "c")
	c := castOn(t, d)
	var mu sync.Mutex
	var got []string
	seen := make(chan struct{}, 10)
	if err := c.Start(func(f screenpipe.Frame) {
		mu.Lock()
		got = append(got, string(f.JPEG))
		mu.Unlock()
		if f.Width != 420 || f.Height != 912 {
			t.Errorf("frame size %dx%d, want the screen in points", f.Width, f.Height)
		}
		seen <- struct{}{}
	}, func() { t.Error("ended") }); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		select {
		case <-seen:
		case <-time.After(5 * time.Second):
			t.Fatal("no frame")
		}
	}
	c.Stop()
	mu.Lock()
	if want := []string{"a", "b", "c"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("frames %v, want %v: a frame like the last is not sent", got, want)
	}
	mu.Unlock()
	time.Sleep(20 * time.Millisecond) // a frame asked before the stop may still land
	n := d.frames()
	time.Sleep(50 * time.Millisecond)
	if d.frames() != n {
		t.Error("the device is still asked for frames after Stop")
	}
}

func TestADeviceWhoseDriverIsGoneEnds(t *testing.T) {
	d := newDriver(t, "a")
	c := castOn(t, d)
	ended := make(chan struct{})
	if err := c.Start(func(screenpipe.Frame) {}, func() { close(ended) }); err != nil {
		t.Fatal(err)
	}
	d.l.Close()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("a source whose driver went away did not end")
	}
	gone := make(chan struct{})
	if err := castOn(t, d).Start(func(screenpipe.Frame) { t.Error("a frame from a gone driver") }, func() { close(gone) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("a source started on a gone driver did not end")
	}
}

func TestAPersonsTouchesAndTypingGoThroughTheDriver(t *testing.T) {
	d := newDriver(t, "a")
	c := castOn(t, d)
	if err := c.Input(screenpipe.Input{Kind: "text", Text: "x"}); err != nil {
		t.Fatal(err) // queued: done once it starts
	}
	if err := c.Start(func(screenpipe.Frame) {}, func() {}); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	for _, ev := range []screenpipe.Input{
		{Kind: "mouse", Action: "move", X: 1, Y: 1},
		{Kind: "mouse", Action: "down", Button: "left", X: 100, Y: 200},
		{Kind: "mouse", Action: "up", Button: "left", X: 104, Y: 203},
		{Kind: "mouse", Action: "down", Button: "left", X: 200, Y: 700},
		{Kind: "mouse", Action: "up", Button: "left", X: 200, Y: 300},
		{Kind: "mouse", Action: "up", Button: "left", X: 5, Y: 5},
		{Kind: "wheel", X: 10, Y: 10, DY: 100},
		{Kind: "key", Action: "down", Key: "h", Text: "h"},
		{Kind: "key", Action: "up", Key: "h"},
		{Kind: "key", Action: "down", Key: "Enter", Text: "\r"},
		{Kind: "key", Action: "down", Key: "Backspace"},
		{Kind: "key", Action: "down", Key: "c", Text: "c", Modifiers: 4},
		{Kind: "key", Action: "down", Key: "Shift"},
		{Kind: "text", Text: "hello"},
	} {
		if err := c.Input(ev); err != nil {
			t.Fatalf("%+v: %v", ev, err)
		}
	}
	want := []string{`{"op":"type","text":"x"}`, `{"op":"tap","x":100,"y":200}`, `{"op":"swipe","x1":200,"x2":200,"y1":700,"y2":300}`, `{"op":"type","text":"h"}`,
		`{"op":"type","text":"\n"}`, `{"op":"type","text":"\b"}`, `{"op":"type","text":"hello"}`}
	deadline := time.Now().Add(5 * time.Second)
	for len(d.got()) < len(want) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := d.got()
	if len(got) != len(want) {
		t.Fatalf("ops %v, want %v", got, want)
	}
	for i, op := range got {
		if b, _ := json.Marshal(op); string(b) != want[i] {
			t.Errorf("op %d = %s, want %s", i, b, want[i])
		}
	}
	c.Stop()
	if err := c.Input(screenpipe.Input{Kind: "text", Text: "late"}); err == nil {
		t.Error("input after Stop was taken")
	}
}
