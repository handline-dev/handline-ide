package browser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTreeShowsRolesNamesAndRefs(t *testing.T) {
	var nodes []axNode
	if err := json.Unmarshal([]byte(`[
		{"nodeId":"1","role":{"value":"RootWebArea"},"name":{"value":"Home"},"childIds":["2","5"],"backendDOMNodeId":3},
		{"nodeId":"2","parentId":"1","role":{"value":"generic"},"name":{"value":""},"childIds":["3"],"backendDOMNodeId":4},
		{"nodeId":"3","parentId":"2","role":{"value":"button"},"name":{"value":"Save"},"childIds":["4"],"backendDOMNodeId":9,
		 "properties":[{"name":"disabled","value":{"value":true}},{"name":"focusable","value":{"value":true}}]},
		{"nodeId":"4","parentId":"3","role":{"value":"StaticText"},"name":{"value":"Save"},"backendDOMNodeId":10},
		{"nodeId":"5","parentId":"1","ignored":true,"childIds":["6"]},
		{"nodeId":"6","parentId":"5","role":{"value":"textbox"},"name":{"value":"Name"},"value":{"value":"abc"},"backendDOMNodeId":12}
	]`), &nodes); err != nil {
		t.Fatal(err)
	}
	byID := map[string]*axNode{}
	for i := range nodes {
		byID[nodes[i].ID] = &nodes[i]
	}
	var out strings.Builder
	write(&out, byID, &nodes[0], 0, "")
	want := `- RootWebArea "Home" [e3]
  - button "Save" disabled [e9]
  - textbox "Name" value="abc" [e12]
`
	if out.String() != want {
		t.Errorf("tree =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestUnzipKeepsModesAndLinks(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "app/bin/run"}
	h.SetMode(0o755)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("#!/bin/sh\n"))
	l := &zip.FileHeader{Name: "app/current"}
	l.SetMode(os.ModeSymlink | 0o777)
	w, _ = z.CreateHeader(l)
	w.Write([]byte("bin"))
	z.Close()
	dir := t.TempDir()
	if err := unzip(bytes.NewReader(buf.Bytes()), int64(buf.Len()), dir); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "app/bin/run")); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("run = %v, %v; want executable", fi, err)
	}
	if to, err := os.Readlink(filepath.Join(dir, "app/current")); err != nil || to != "bin" {
		t.Errorf("current links to %q, %v", to, err)
	}
}

func TestUnzipRefusesAPathOutside(t *testing.T) {
	for _, link := range []string{"", "/", "../.."} {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		if link == "" {
			w, _ := z.Create("../out")
			w.Write([]byte("x"))
		} else {
			h := &zip.FileHeader{Name: "a"}
			h.SetMode(os.ModeSymlink | 0o777)
			w, _ := z.CreateHeader(h)
			w.Write([]byte(link))
		}
		z.Close()
		if err := unzip(bytes.NewReader(buf.Bytes()), int64(buf.Len()), t.TempDir()); err == nil {
			t.Errorf("an entry leaving the folder (link %q) was written", link)
		}
	}
}

// TestDrivesAPageInItsOwnProfile runs the machine's Chrome, so it is skipped on one without.
func TestDrivesAPageInItsOwnProfile(t *testing.T) {
	engine := machineChrome()
	if engine == "" {
		t.Skip("no Chrome on this machine")
	}
	tmp, err := os.MkdirTemp("/tmp", "hlc") // short: Chrome's socket under it must fit a unix socket path; what Chrome leaves goes with it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	t.Setenv("TMPDIR", tmp)
	one, two := t.TempDir(), t.TempDir()
	a, err := Ensure(one, engine, false)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(one)
	b, err := Ensure(two, engine, false)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(two)
	if a.PID == b.PID || a.Port == b.Port {
		t.Fatalf("two folders share one browser: %+v %+v", a, b)
	}
	if again, err := Ensure(one, engine, false); err != nil || again.PID != a.PID {
		t.Fatalf("a second Ensure started another browser: %+v, %v", again, err)
	}
	p, err := a.Page()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	where, err := p.Open(`data:text/html,<title>T</title><input id=n><button onclick="document.title='Hi '+n.value">Go</button>`)
	if err != nil || !strings.HasPrefix(where, "T ") {
		t.Fatalf("open = %q, %v", where, err)
	}
	if err := p.Type("#n", "abc", false); err != nil {
		t.Fatal(err)
	}
	tree, err := p.Tree()
	if err != nil || !strings.Contains(tree, `button "Go" [e`) {
		t.Fatalf("tree = %q, %v", tree, err)
	}
	ref := tree[strings.Index(tree, `button "Go" [`)+len(`button "Go" [`):]
	ref = ref[:strings.Index(ref, "]")]
	if err := p.Click(ref); err != nil {
		t.Fatal(err)
	}
	if title, err := p.Eval("document.title"); err != nil || title != "Hi abc" {
		t.Errorf("title after the click = %q, %v", title, err)
	}
	shot := filepath.Join(t.TempDir(), "s.png")
	if err := p.Shot(shot, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(shot); !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Error("the screenshot is not a PNG")
	}
	if !Close(one) || Running(one) != nil {
		t.Error("Close left the browser running")
	}
	if Running(two) == nil {
		t.Error("closing one agent's browser closed the other's")
	}
}
