package browser

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/testfiles"
)

// testEngine is the machine's Chrome, else the one HANDLINE_TEST_CHROME names; the test is skipped without either.
func testEngine(t *testing.T) string {
	if e := machineChrome(); e != "" {
		return e
	}
	if e := os.Getenv("HANDLINE_TEST_CHROME"); e != "" {
		return e
	}
	t.Skip("no Chrome on this machine")
	return ""
}

// testBrowser is a browser of the test's own, closed when it ends.
func testBrowser(t *testing.T) *Browser {
	engine := testEngine(t)
	tmp, err := os.MkdirTemp("/tmp", "hlc") // short: Chrome's socket under it must fit a unix socket path; what Chrome leaves goes with it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	t.Setenv("TMPDIR", tmp)
	dir := t.TempDir()
	b, err := Ensure(dir, engine, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Close(dir) })
	return b
}

const testPage = `<!doctype html><title>Actions</title>
<script>console.log("hello", 2); addEventListener("keydown", e => (window.keys ||= []).push(e.key));</script>
<img src="/missing.png">
<input id=k>
<select id=s><option value=r>Red</option><option value=g>Green</option></select>
<div id=hover onmouseover="this.textContent='hovered'" style="width:80px;height:30px">h</div>
<div id=d1 draggable=true ondragstart="event.dataTransfer.setData('text/plain','one')" style="width:60px;height:30px">drag</div>
<div id=drop ondragover="event.preventDefault()" ondrop="this.textContent='got '+event.dataTransfer.getData('text/plain')"
 style="width:80px;height:40px">drop here</div>
<div id=p style="width:40px;height:30px;background:#ccc"
 onpointerdown="window.down=[event.clientX,event.clientY]">p</div>
<script>addEventListener("pointerup", e => window.up = [e.clientX, e.clientY])</script>
<button id=ask onclick="window.r=confirm('sure?')">ask</button>
<input id=f type=file>
<a id=dl href="/file">get</a>
<div style="height:3000px"></div>`

// TestKeeperProcess is the process Resize starts, when a test asks for it.
func TestKeeperProcess(t *testing.T) {
	if os.Getenv("HANDLINE_TEST_KEEPER") == "" {
		return
	}
	args := flag.Args()
	w, _ := strconv.Atoi(args[2])
	h, _ := strconv.Atoi(args[3])
	err := kept(args[0]).Keep(args[1], w, h, func() { fmt.Println("holding") })
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func testSite(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(testPage))
		case "/two":
			w.Write([]byte(`<title>Two</title><p>two`))
		case "/asks":
			w.Write([]byte(`<title>Asks</title><script>window.r = confirm("at load")</script>`))
		case "/file":
			w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
			w.Write([]byte("the report"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func page(t *testing.T, b *Browser) *Page {
	t.Helper()
	p, err := b.Page()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// until waits for ok, up to 10 seconds.
func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	testfiles.Until(t, what, 10*time.Second, 20*time.Millisecond, ok)
}

func evalIs(t *testing.T, p *Page, script, want string) {
	t.Helper()
	if got, err := p.Eval(script); err != nil || got != want {
		t.Errorf("%s = %q, %v; want %q", script, got, err, want)
	}
}

func box(t *testing.T, p *Page, target string) Box {
	t.Helper()
	b, err := p.Box(target)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEachActionOnALocalPage(t *testing.T) {
	b := testBrowser(t)
	site := testSite(t)
	open := func(t *testing.T, path string) *Page {
		p := page(t, b)
		if _, err := p.Open(site.URL + path); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("press", func(t *testing.T) {
		p := open(t, "/")
		if err := p.TypeAt(box(t, p, "#k"), "ab", false); err != nil {
			t.Fatal(err)
		}
		press := func(ks ...string) {
			for _, k := range ks {
				if err := p.Press(k); err != nil {
					t.Fatalf("press %s: %v", k, err)
				}
			}
		}
		press("Shift+c", "Backspace", "d")
		evalIs(t, p, "k.value", "abd")
		press("Meta+a", "x", "Escape", "ArrowLeft", "Tab")
		evalIs(t, p, "k.value", "x")
		evalIs(t, p, "keys.join(' ')", "C Backspace d a x Escape ArrowLeft Tab")
		evalIs(t, p, "document.activeElement.id", "s")
		if err := p.Press("Hyper+a"); err == nil || !strings.Contains(err.Error(), "not a modifier") {
			t.Errorf("an unknown modifier: %v", err)
		}
	})

	t.Run("select", func(t *testing.T) {
		p := open(t, "/")
		p.Eval(`s.onchange = () => window.changed = s.value; ""`)
		if label, err := p.Select("#s", "Green"); err != nil || label != "Green" {
			t.Fatalf("select by label = %q, %v", label, err)
		}
		evalIs(t, p, "changed", "g")
		if label, err := p.Select("#s", "r"); err != nil || label != "Red" {
			t.Errorf("select by value = %q, %v", label, err)
		}
		if _, err := p.Select("#s", "Blue"); err == nil || !strings.Contains(err.Error(), `"Red", "Green"`) {
			t.Errorf("a missing option: %v", err)
		}
		if _, err := p.Select("#k", "x"); err == nil || !strings.Contains(err.Error(), "not a select") {
			t.Errorf("not a select: %v", err)
		}
	})

	t.Run("hover", func(t *testing.T) {
		p := open(t, "/")
		if err := p.HoverAt(box(t, p, "#hover")); err != nil {
			t.Fatal(err)
		}
		evalIs(t, p, "document.querySelector('#hover').textContent", "hovered")
	})

	t.Run("drag", func(t *testing.T) {
		p := open(t, "/")
		from, onto, err := p.DragTo("#d1", "#drop")
		if err != nil {
			t.Fatal(err)
		}
		if err := p.DragAt(from, onto); err != nil {
			t.Fatal(err)
		}
		evalIs(t, p, "drop.textContent", "got one")
		from, onto, _ = p.DragTo("#p", "#hover")
		if err := p.DragAt(from, onto); err != nil {
			t.Fatal(err)
		}
		x, y := onto.centre()
		evalIs(t, p, "JSON.stringify([down.length, up.map(Math.round)])", fmt.Sprintf("[2,[%.0f,%.0f]]", x, y))
	})

	t.Run("scroll", func(t *testing.T) {
		p := open(t, "/")
		v, err := p.Viewport()
		if err != nil {
			t.Fatal(err)
		}
		at, err := p.ScrollAt(v, 0, 500)
		if err != nil || !strings.HasPrefix(at, "at 0,500 of 0,") {
			t.Errorf("scroll = %q, %v", at, err)
		}
	})

	t.Run("dialogs", func(t *testing.T) {
		p := open(t, "/")
		if err := p.ClickAt(box(t, p, "#ask")); err != nil {
			t.Fatal(err)
		}
		p.Settle()
		if want := []string{`confirm "sure?" dismissed`}; !slices.Equal(p.Asked, want) {
			t.Errorf("asked = %q, want %q", p.Asked, want)
		}
		evalIs(t, p, "String(r)", "false")
		p.Close()
		if err := b.SetDialogs(true, ""); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.SetDialogs(false, "") })
		p = page(t, b) // hl browser dialog tells the page too
		p.Eval(`setTimeout(() => alert("later"), 0); ""`)
		p.Close()
		var again *Page // a timer's dialog, between commands: the first command after it says what it asked
		until(t, "the timer's alert", func() bool {
			if again != nil {
				again.Close()
			}
			var err error
			if again, err = b.Page(); err != nil {
				t.Fatal(err)
			}
			return len(again.Asked) > 0
		})
		t.Cleanup(again.Close)
		if want := []string{`alert "later" accepted`}; !slices.Equal(again.Asked, want) {
			t.Errorf("asked between commands = %q, want %q", again.Asked, want)
		}
		if err := again.ClickAt(box(t, again, "#ask")); err != nil {
			t.Fatal(err)
		}
		evalIs(t, again, "String(r)", "true")
		if _, err := again.Open(site.URL + "/asks"); err != nil { // a new document's own dialog
			t.Fatal(err)
		}
		again.Settle()
		if want := []string{`alert "later" accepted`, `confirm "at load" accepted`}; !slices.Equal(again.Asked, want) {
			t.Errorf("asked = %q, want %q", again.Asked, want)
		}
		evalIs(t, again, "String(r)", "true")
	})

	t.Run("history", func(t *testing.T) {
		p := open(t, "/")
		if _, err := p.Open(site.URL + "/two"); err != nil {
			t.Fatal(err)
		}
		if w, err := p.Go(-1); err != nil || !strings.HasPrefix(w, "Actions ") {
			t.Errorf("back = %q, %v", w, err)
		}
		if w, err := p.Go(1); err != nil || !strings.HasPrefix(w, "Two ") {
			t.Errorf("forward = %q, %v", w, err)
		}
		if _, err := p.Go(1); err == nil || err.Error() != "there is no page to go forward to" {
			t.Errorf("forward at the end: %v", err)
		}
		p.Eval(`window.mark = 1; ""`)
		if w, err := p.Reload(); err != nil || !strings.HasPrefix(w, "Two ") {
			t.Errorf("reload = %q, %v", w, err)
		}
		evalIs(t, p, "String(window.mark)", "undefined")
	})

	t.Run("tabs", func(t *testing.T) {
		open(t, "/").Close()
		if err := b.TabNew(); err != nil {
			t.Fatal(err)
		}
		tabs, err := b.TabList()
		if err != nil || len(tabs) != 2 || !tabs[1].Current || tabs[0].Title != "Actions" {
			t.Fatalf("tabs after new = %+v, %v", tabs, err)
		}
		evalIs(t, page(t, b), "location.href", "about:blank")
		if err := b.TabTo(1); err != nil {
			t.Fatal(err)
		}
		evalIs(t, page(t, b), "document.title", "Actions")
		if err := b.TabClose(1); err != nil {
			t.Fatal(err)
		}
		if tabs, _ := b.TabList(); len(tabs) != 1 || tabs[0].URL != "about:blank" || !tabs[0].Current {
			t.Errorf("tabs after close = %+v", tabs)
		}
		if err := b.TabTo(2); err == nil {
			t.Error("a tab that is not there was moved to")
		}
	})

	t.Run("viewport", func(t *testing.T) {
		p := open(t, "/")
		keeper := []string{os.Args[0], "-test.run=^TestKeeperProcess$", "--", b.Dir}
		t.Setenv("HANDLINE_TEST_KEEPER", "1")
		for _, size := range [][2]int{{700, 500}, {390, 844}} {
			if err := p.Resize(size[0], size[1], keeper...); err != nil {
				t.Fatal(err)
			}
		}
		p.Close()
		later := page(t, b) // the size outlives the command that set it
		if v, err := later.Viewport(); err != nil || v.W != 390 || v.H != 844 {
			t.Errorf("viewport = %+v, %v", v, err)
		}
		id := b.PageID
		pid, _ := os.ReadFile(keeperFile(b.Dir, id))
		if err := b.TabNew(); err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(string(pid))
		if !keeping(n, id) {
			t.Fatal("nothing holds the size")
		}
		if err := b.TabClose(1); err != nil {
			t.Fatal(err)
		}
		for end := time.Now().Add(5 * time.Second); keeping(n, id); time.Sleep(50 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatal("what held the size outlived its tab")
			}
		}
	})

	t.Run("upload", func(t *testing.T) {
		p := open(t, "/")
		file := filepath.Join(t.TempDir(), "up.txt")
		os.WriteFile(file, []byte("x"), 0o600)
		if err := p.Upload("#f", []string{file}); err != nil {
			t.Fatal(err)
		}
		evalIs(t, p, "f.files[0].name", "up.txt")
		if err := p.Upload("#k", []string{file}); err == nil || !strings.Contains(err.Error(), "not a file input") {
			t.Errorf("upload to a text field: %v", err)
		}
	})

	t.Run("download", func(t *testing.T) {
		p := open(t, "/")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "report.txt"), []byte("mine"), 0o600)
		file, err := p.Download(box(t, p, "#dl"), dir, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(file); file != filepath.Join(dir, "report-2.txt") || string(got) != "the report" {
			t.Errorf("downloaded %s holding %q", file, got)
		}
		if _, err := p.Download(box(t, p, "#k"), dir, time.Second); err == nil || !strings.Contains(err.Error(), "no download") {
			t.Errorf("a click that downloads nothing: %v", err)
		}
	})

	t.Run("console and network", func(t *testing.T) {
		open(t, "/").Close()
		p := page(t, b)
		out, err := p.Console()
		if err != nil || !strings.Contains(out, "log  hello 2") || !strings.Contains(out, "error  Failed to load resource") {
			t.Errorf("console = %q, %v", out, err)
		}
		net, err := p.Network()
		if err != nil || !strings.HasPrefix(net, "200  navigation  ") || !strings.Contains(net, "404  img  ") {
			t.Errorf("network = %q, %v", net, err)
		}
	})
}
