package supervisor

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/taskboard/attachments"
)

func TestAnAgentIsToldWhereTheImagesOfATasksTextsAre(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	s := attachments.New(r.home)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	put := func() attachments.Info {
		out, err := s.Put(r.DB, attachments.PutIn{Name: "a.png", Mime: "image/png", Bytes: int64(len(png)), Data: png})
		if err != nil || out.Attachment == nil {
			t.Fatalf("put: %v %v", out, err)
		}
		return *out.Attachment
	}
	brief, comment := put(), put()
	line := func(n int, i attachments.Info) string {
		return attachments.Line(n, s.Path(i), "image/png", int64(len(png)))
	}
	tid := r.newTask("bob", "alice", "job")
	_, err := db.AddComment(r.DB, tid, "alice", "this one ![m](attachment:"+comment.ID+")")
	r.must(err)
	rid := r.ask("alice", "bob", "fix it ![b](attachment:"+brief.ID+")", "task")
	r.must(db.LinkRequest(r.DB, tid, rid))
	if got := r.imageLines(r.DB, mustReq(r, rid)); got != line(1, brief)+"\n"+line(2, comment) {
		t.Fatalf("the brief's image, then its comments': %q", got)
	}
	if got := r.imageLines(r.DB, mustReq(r, r.ask("alice", "bob", "![b](attachment:"+brief.ID+")", "say"))); got != line(1, brief) {
		t.Fatalf("a say names images too: %q", got)
	}
	r.AdvanceCurrentTask(r.DB, "bob")
	r.term.typed = nil
	r.must(r.TellComment(r.DB, tid, "alice", "added", strings.Repeat("w", 420)+" ![m](attachment:"+comment.ID+")", "", true))
	if !strings.Contains(r.term.last(), line(1, comment)) {
		t.Fatalf("a long comment still names its image: %q", r.term.last())
	}
}

func TestAnAnswerCarriesOnlyTheImagesOfTheCommentsItBrings(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	s := attachments.New(r.home)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	out, err := s.Put(r.DB, attachments.PutIn{Name: "a.png", Mime: "image/png", Bytes: int64(len(png)), Data: png})
	if err != nil || out.Attachment == nil {
		t.Fatalf("put: %v %v", out, err)
	}
	old := *out.Attachment
	tid := r.newTask("bob", "alice", "job")
	_, err = db.AddComment(r.DB, tid, "p1", "look at this ![x](attachment:"+old.ID+")")
	r.must(err)
	_, err = db.AddComment(r.DB, tid, r.k("bob"), "A, B or C?")
	r.must(err)
	answer, err := db.AddComment(r.DB, tid, "p1", "OK")
	r.must(err)
	rid := r.ask("alice", "bob", "T1 댓글 2 질문에 대표님 댓글 3: OK", "task")
	r.must(db.LinkRequest(r.DB, tid, rid))
	r.must(db.LinkCommentTell(r.DB, answer, r.k("bob"), rid))
	if got := r.imageLines(r.DB, mustReq(r, rid)); got != "" {
		t.Fatalf("an answer with no image carries the older comment's: %q", got)
	}
	_, err = db.AddComment(r.DB, tid, "p1", "and this ![x](attachment:"+old.ID+")")
	r.must(err)
	if got := r.imageLines(r.DB, mustReq(r, rid)); got != attachments.Line(1, s.Path(old), "image/png", int64(len(png))) {
		t.Fatalf("a comment the delivery shows names its image: %q", got)
	}
	withImage, err := db.AddComment(r.DB, tid, "p1", "this ![x](attachment:"+old.ID+")")
	r.must(err)
	rid = r.ask("alice", "bob", "T1 댓글 2 질문에 대표님 댓글 5: this", "task")
	r.must(db.LinkRequest(r.DB, tid, rid))
	r.must(db.LinkCommentTell(r.DB, withImage, r.k("bob"), rid))
	_, err = db.AddComment(r.DB, tid, r.k("bob"), "thanks")
	r.must(err)
	if got := r.imageLines(r.DB, mustReq(r, rid)); got != attachments.Line(1, s.Path(old), "image/png", int64(len(png))) {
		t.Fatalf("the answer's own image is carried: %q", got)
	}
}
