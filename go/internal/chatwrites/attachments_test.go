package chatwrites

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/access"
	"github.com/handline/handline/go/taskboard/attachments"
)

func putImage(t *testing.T, r *rig) *attachments.Info {
	t.Helper()
	b := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 2000))
	out, err := attachments.New(r.home).Put(r.db, attachments.PutIn{Name: "shot.png", Mime: "image/png", Bytes: int64(len(b)), Data: b})
	if err != nil || out.Attachment == nil {
		t.Fatal(err)
	}
	return out.Attachment
}

func TestSendNamesTheImagesAndTheMessageKeepsThem(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	img := putImage(t, r)
	res := r.result(r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":"look","attachments":[%q],"request_id":"a1"}`, img.ID)))
	rid := int64(res["request"].(float64))
	got, err := attachments.Of(r.db, rid)
	if err != nil || len(got) != 1 || got[0] != *img {
		t.Fatalf("the message carries %v %v", got, err)
	}
	line := attachments.New(r.home).MessageLines(r.db, rid, "look")
	want := fmt.Sprintf("[image 1: %s/attachments/%s.png (image/png, 2 KB)]", r.home, img.ID)
	if line != want {
		t.Fatalf("path line %q, want %q", line, want)
	}
	var body string
	r.db.QueryRow("SELECT body FROM requests WHERE id = ?", rid).Scan(&body)
	if body != "look" {
		t.Fatalf("the body stays as typed: %q", body)
	}
}

func TestImagesMayBeTheWholeMessage(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	img := putImage(t, r)
	r.result(r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","attachments":[%q],"request_id":"a2"}`, img.ID)))
	e, _ := r.one(s, "chat.send", `{"agent":"alice","request_id":"a3"}`)["error"].(map[string]any)
	if e["data"].(map[string]any)["field"] != "body" {
		t.Fatalf("no body and no image: %v", e)
	}
}

func TestSendRefusesWhatIsNotAnImageList(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	img := putImage(t, r)
	for i, params := range []string{
		`"attachments":[]`,
		`"attachments":"x"`,
		`"attachments":[1]`,
		`"attachments":["../x"]`,
		`"attachments":["0000000000000001","0000000000000002","0000000000000003","0000000000000004","0000000000000005","0000000000000006","0000000000000007"]`,
		fmt.Sprintf(`"task":{"title":"t"},"attachments":[%q]`, img.ID),
	} {
		e, _ := r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":"x",%s,"request_id":"b%d"}`, params, i))["error"].(map[string]any)
		if e == nil || e["data"].(map[string]any)["field"] != "attachments" {
			t.Fatalf("%s: %v", params, e)
		}
	}
	e, _ := r.one(s, "chat.send", `{"agent":"alice","body":"x","attachments":["0000000000000000"],"request_id":"c1"}`)["error"].(map[string]any)
	if e["data"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("an image that is not there: %v", e)
	}
	if n := r.count("SELECT COUNT(*) FROM requests"); n != 0 {
		t.Fatalf("a refused send wrote %d requests", n)
	}
}

func TestASendsBodyNamesImagesAsATasksTextDoes(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	a, b := putImage(t, r), putImage(t, r)
	body := fmt.Sprintf("see ![the button](attachment:%s)", a.ID)
	res := r.result(r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":%q,"attachments":[%q,%q],"request_id":"d1"}`, body, b.ID, a.ID)))
	rid := int64(res["request"].(float64))
	if got, _ := attachments.Of(r.db, rid); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("only the listed id the body does not name is recorded: %v", got)
	}
	lines := attachments.New(r.home).MessageLines(r.db, rid, body)
	if !strings.HasPrefix(lines, "[image 1: "+r.home+"/attachments/"+a.ID) || !strings.Contains(lines, "[image 2: "+r.home+"/attachments/"+b.ID) {
		t.Fatalf("the reference first, then the listed: %q", lines)
	}
	r.result(r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":"again","attachments":[%q],"request_id":"d2"}`, a.ID)))
	var refs string
	for _, img := range []*attachments.Info{a, b, putImage(t, r), putImage(t, r)} {
		refs += fmt.Sprintf("![x](attachment:%s)", img.ID)
	}
	more := []*attachments.Info{putImage(t, r), putImage(t, r), putImage(t, r)}
	e, _ := r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":%q,"attachments":[%q,%q,%q],"request_id":"d3"}`, refs, more[0].ID, more[1].ID, more[2].ID))["error"].(map[string]any)
	if e == nil || e["data"].(map[string]any)["field"] != "attachments" {
		t.Fatalf("4 referenced and 3 listed are over 6: %v", e)
	}
	e, _ = r.one(s, "chat.send", `{"agent":"alice","body":"![x](attachment:0000000000000000)","request_id":"d4"}`)["error"].(map[string]any)
	if e == nil || e["data"].(map[string]any)["what"] != "attachment" {
		t.Fatalf("a reference to an image that is not there: %v", e)
	}
	res = r.result(r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":%q,"task":{"title":"fix it"},"request_id":"d5"}`, body)))
	if res["accepted"] != true {
		t.Fatalf("a message that opens a task may name images: %v", res)
	}
	held, _ := attachments.Held(r.db, 1)
	if _, ok := held[a.ID]; !ok {
		t.Fatalf("the task's brief holds the image: %v", held)
	}
}

func TestAnAgentSaysOnlyImagesAMessageOrTaskAlreadyShows(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	founder, alice := r.server(""), r.server("alice")
	sent, fresh := putImage(t, r), putImage(t, r)
	r.result(r.one(founder, "chat.send", fmt.Sprintf(`{"agent":"alice","body":"look","attachments":[%q],"request_id":"e1"}`, sent.ID)))
	e, _ := r.one(alice, "say", fmt.Sprintf(`{"target":"bob","body":"![x](attachment:%s)","request_id":"e2"}`, fresh.ID))["error"].(map[string]any)
	if e == nil || e["data"].(map[string]any)["gate"] != "role" {
		t.Fatalf("an upload no one has sent: %v", e)
	}
	r.result(r.one(alice, "say", fmt.Sprintf(`{"target":"bob","body":"the founder's ![x](attachment:%s)","request_id":"e3"}`, sent.ID)))
}

func TestAReplyNamesOnlyImagesThatAreThereAndTheAgentMayShow(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	alice := r.server("alice")
	fresh := putImage(t, r)
	item := r.request("bob", "alice", "question")
	if d := r.errData(r.one(alice, "inbox.reply", fmt.Sprintf(`{"id":%d,"text":"![x](attachment:%s)","request_id":"f1"}`, item, fresh.ID))); d["gate"] != "role" {
		t.Fatalf("an upload no one has sent: %v", d)
	}
	if d := r.errData(r.one(alice, "inbox.reply", fmt.Sprintf(`{"id":%d,"text":"![x](attachment:0000000000000000)","request_id":"f2"}`, item))); d["what"] != "attachment" {
		t.Fatalf("a reference to an image that is not there: %v", d)
	}
	if n := r.count("SELECT COUNT(*) FROM requests WHERE settles = ?", item); n != 0 {
		t.Fatalf("a refused reply wrote %d replies", n)
	}
}

func TestAMessageThatOpensATaskIsRefusedForAGoneImage(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	e, _ := r.one(r.server(""), "chat.send", `{"agent":"alice","body":"![x](attachment:0000000000000000)","task":{"title":"fix it"},"request_id":"g1"}`)["error"].(map[string]any)
	if e == nil || e["data"].(map[string]any)["what"] != "attachment" {
		t.Fatalf("a brief naming an image that is not there: %v", e)
	}
	if n := r.count("SELECT COUNT(*) FROM tasks"); n != 0 {
		t.Fatalf("a refused send opened %d tasks", n)
	}
}

func TestSayCarriesTheImageFilesItNamesAndRefusesAnythingElse(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	dir := t.TempDir()
	png := dir + "/shot.png"
	os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n"+strings.Repeat("x", 2000)), 0o644)
	os.WriteFile(dir+"/notes.md", []byte("# hi"), 0o644)
	os.WriteFile(dir+"/fake.png", []byte("not a png"), 0o644)
	res := r.result(r.one(s, "say", fmt.Sprintf(`{"target":"alice","body":"look","files":[%q],"request_id":"s1"}`, png)))
	if res["accepted"] != true {
		t.Fatalf("%v", res)
	}
	var rid int64
	r.db.QueryRow("SELECT MAX(id) FROM requests").Scan(&rid)
	got, err := attachments.Of(r.db, rid)
	if err != nil || len(got) != 1 || got[0].Mime != "image/png" || got[0].Name != "shot.png" {
		t.Fatalf("the say carries %v %v", got, err)
	}
	if maps := attachments.New(r.home).MessageMaps(r.db, rid, "look"); len(maps) != 1 {
		t.Fatalf("chat.messages shows %v", maps)
	}
	for i, f := range []string{dir + "/notes.md", dir + "/fake.png"} {
		d := r.errData(r.one(s, "say", fmt.Sprintf(`{"target":"alice","body":"x","files":[%q],"request_id":"s%d"}`, f, i+2)))
		if d["field"] != "files" {
			t.Fatalf("%s: %v", f, d)
		}
	}
	if n := r.count("SELECT COUNT(*) FROM requests WHERE body = 'x'"); n != 0 {
		t.Fatalf("a refused say wrote %d messages", n)
	}
}

// A path ref is kept with the message as sent and told to the agent after the attachments, as a file or a folder; refs count with the
// attachments; only the founder sends them.
func TestSendKeepsPathRefsAndTellsThemAfterTheAttachments(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	img := putImage(t, r)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("a"), 0o600)
	res := r.result(r.call(s, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"founder"}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"chat.send","params":{"agent":"alice","body":"","attachments":[%q],"refs":[%q,%q],"request_id":"r1"}}`, img.ID, file, dir))[1])
	rid := int64(res["request"].(float64))
	want := fmt.Sprintf("[image 1: %s/attachments/%s.png (image/png, 2 KB)]\n[file 2: %s]\n[folder 3: %s]", r.home, img.ID, file, dir)
	if line := attachments.New(r.home).MessageLines(r.db, rid, ""); line != want {
		t.Fatalf("%q, want %q", line, want)
	}
	if got := attachments.PathRefsOf(r.db, rid); len(got) != 2 || got[0].Path != file || got[1].Path != dir {
		t.Fatalf("%v", got)
	}
	member := &rpc.Conn{Caller: rpc.Caller{Role: "member", Kind: "device", Info: map[string]any{"kind": "device", "name": "phone"}}}
	if _, err := refPaths(member, map[string]any{"refs": []any{dir}}, 0); err == nil || err.(*rpc.Error).Data["gate"] != "role" {
		t.Fatalf("a caller not proved the founder: %v", err)
	}
	founder := &rpc.Conn{Caller: rpc.Caller{Role: "founder"}}
	for name, p := range map[string]map[string]any{
		"seven with the attachments": {"refs": []any{"/a", "/b"}}, "with a task": {"refs": []any{"/a"}, "task": map[string]any{}},
		"empty": {"refs": []any{}}, "twice": {"refs": []any{"/a", "/a"}}, "not text": {"refs": []any{5}}, "not a list": {"refs": "/a"},
	} {
		n := 0
		if name == "seven with the attachments" {
			n = 5
		}
		if _, err := refPaths(founder, p, n); err == nil || err.(*rpc.Error).Data["field"] != "refs" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got, err := refPaths(founder, map[string]any{"refs": []any{"/a"}}, 5); err != nil || len(got) != 1 {
		t.Fatalf("six in all: %v %v", got, err)
	}
	// The images the body names count too.
	named := fmt.Sprintf("![a](attachment:%s)", img.ID)
	for i := 0; i < 5; i++ {
		named += fmt.Sprintf(" ![x](attachment:%s)", putImage(t, r).ID)
	}
	e, _ := r.call(s, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"founder"}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"chat.send","params":{"agent":"alice","body":%q,"refs":[%q],"request_id":"r3"}}`, named, dir))[1]["error"].(map[string]any)
	if e == nil || e["data"].(map[string]any)["field"] != "refs" {
		t.Fatalf("six named images and a ref: %v", e)
	}
	if _, err := attachments.CheckPathRefs([]string{dir + "/a\nb"}); err == nil {
		t.Fatal("a path on two lines")
	}
}

func TestCheckPathRefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for p, code := range map[string]string{"rel/a": "bad_request", dir + "/../x": "bad_request", dir + "/nope": "not_found"} {
		_, err := attachments.CheckPathRefs([]string{p})
		if e, ok := err.(*rpc.Error); !ok || e.Code != code || (code == "not_found" && e.Data["what"] != "path") {
			t.Errorf("%s: %v", p, err)
		}
	}
	if refs, err := attachments.CheckPathRefs([]string{dir}); err != nil || !refs[0].Folder {
		t.Fatalf("%v %v", refs, err)
	}
}

// With chat_refs_need_grant on, a ref outside what the agent may read is refused, gate access_needed, naming the agent and the paths
// outside, and nothing is written; a grant, or a ref in the agent's own folder, is sent. Off, any ref is sent.
func TestSendRefsNeedAGrantWithTheSettingOn(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	s := r.server("")
	dir, own := t.TempDir(), paths.AgentDir(r.home, "alice")
	send := func(rid string, refs ...string) map[string]any {
		b, _ := json.Marshal(refs)
		return r.one(s, "chat.send", fmt.Sprintf(`{"agent":"alice","body":"see","refs":%s,"request_id":%q}`, b, rid))
	}
	if m := send("off", dir); m["error"] != nil {
		t.Fatalf("setting off: %v", m)
	}
	os.WriteFile(filepath.Join(r.home, "config.yaml"), []byte("chat_refs_need_grant: true\n"), 0o644)
	before := r.count("SELECT COUNT(*) FROM requests")
	e, _ := send("on", own, dir)["error"].(map[string]any)
	if d, _ := e["data"].(map[string]any); d == nil || d["gate"] != "access_needed" || d["agent"] != r.k("alice") || fmt.Sprint(d["paths"]) != fmt.Sprint([]any{dir}) {
		t.Fatalf("%v", e)
	}
	if n := r.count("SELECT COUNT(*) FROM requests"); n != before {
		t.Fatalf("a refused send wrote %d requests", n-before)
	}
	access.Put(r.db, r.k("alice"), dir, "once", 1)
	if m := send("on", own, dir); m["error"] != nil {
		t.Fatalf("granted: %v", m)
	}
}
