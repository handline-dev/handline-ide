package browser

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/screenpipe"
)

func TestCastSendsFramesTakesInputAndEmptiesWhatAPersonTyped(t *testing.T) {
	engine := testEngine(t)
	tmp, err := os.MkdirTemp("/tmp", "hlc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	t.Setenv("TMPDIR", tmp)
	dir := t.TempDir()
	if _, err := OpenCast(dir); err != screenpipe.ErrNotOpen {
		t.Fatalf("a folder with no browser: %v", err)
	}
	b, err := Ensure(dir, engine, false)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(dir)
	p, err := b.Page()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Open(`data:text/html,<form onsubmit="return false"><input type=password id=mine><input type=password id=agents></form>` +
		`<input id=n><button id=ask onclick="const f = document.createElement('iframe'); f.srcdoc = '<script>parent.r = confirm(1)</sc'+'ript>'; document.body.append(f)">ask</button>`); err != nil {
		t.Fatal(err)
	}
	box, err := p.Box("#agents")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.TypeAt(box, "agent-typed", false); err != nil {
		t.Fatal(err)
	}
	cast, err := OpenCast(dir)
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan screenpipe.Frame, 100)
	if err := cast.Start(func(f screenpipe.Frame) { frames <- f }, func() {}); err != nil {
		t.Fatal(err)
	}
	defer cast.Stop()
	select {
	case f := <-frames:
		if !bytes.HasPrefix(f.JPEG, []byte{0xff, 0xd8}) || f.Width == 0 || f.Height == 0 {
			t.Errorf("the first frame: %d bytes, %dx%d", len(f.JPEG), f.Width, f.Height)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no frame")
	}
	mine, err := p.Box("#mine")
	if err != nil {
		t.Fatal(err)
	}
	x, y := mine.centre()
	for _, ev := range []screenpipe.Input{
		{Kind: "mouse", Action: "down", X: x, Y: y, Button: "left"}, {Kind: "mouse", Action: "up", X: x, Y: y, Button: "left"},
		{Kind: "key", Action: "down", Key: "a", Code: "KeyA", Text: "a"}, {Kind: "key", Action: "up", Key: "a", Code: "KeyA"},
		{Kind: "text", Text: "bc"},
	} {
		if err := cast.Input(ev); err != nil {
			t.Fatal(err)
		}
	}
	if v, err := p.Eval("document.querySelector('#mine').value"); err != nil || v != "abc" {
		t.Fatalf("what the person typed = %q, %v", v, err)
	}
	if err := cast.Release(); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.Eval("[document.querySelector('#mine').value, document.querySelector('#agents').value].join('|')"); v != "|agent-typed" {
		t.Errorf("after the release: %q; want the person's field empty and the agent's kept", v)
	}
	ask, err := p.Box("#ask")
	if err != nil {
		t.Fatal(err)
	}
	p.Close() // what the commands gave the page answers no dialog of a frame made after
	x, y = ask.centre()
	for _, a := range []string{"down", "up"} {
		if err := cast.Input(screenpipe.Input{Kind: "mouse", Action: a, X: x, Y: y, Button: "left"}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(200 * time.Millisecond)
	again, err := b.Page()
	if err != nil {
		t.Fatalf("the dialog a person's click opened holds the page: %v", err)
	}
	defer again.Close()
	if v, err := again.Eval("String(window.r)"); err != nil || v != "false" {
		t.Errorf("the dialog was answered %q, %v; want dismissed", v, err)
	}
}
