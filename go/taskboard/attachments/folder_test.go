package attachments

import (
	"os"
	"strings"
	"path/filepath"
	"testing"
	"time"
)

// Files put into a folder sit at their rel_path under its directory, and the folder counts them; the agent is told the directory.
func TestAFolderHoldsItsFilesAtTheirPathsAndCountsThem(t *testing.T) {
	s, _, q := testStore(t)
	f, err := s.Folder("notes")
	if err != nil {
		t.Fatal(err)
	}
	if f.Mime != FolderMime || f.Files != 0 || f.Map()["files"] != 0 {
		t.Fatalf("an empty folder: %+v %v", f, f.Map())
	}
	if _, err := s.Put(q, PutIn{Name: "a.md", Mime: "text/plain", Bytes: 6, Data: []byte("alpha\n"), Folder: f.ID, Rel: "a.md"}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Put(q, PutIn{Name: "b.md", Mime: "text/plain", Bytes: 5, Data: []byte("beta\n"), Folder: f.ID, Rel: "sub/b.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Attachment; got.ID != f.ID || got.Files != 2 || got.Bytes != 11 {
		t.Fatalf("the folder after two files: %+v", got)
	}
	b, err := os.ReadFile(filepath.Join(s.Path(*f), "sub", "b.md"))
	if st, _ := os.Stat(filepath.Join(s.Path(*f), "sub", "b.md")); err != nil || string(b) != "beta\n" || st.Mode().Perm() != 0o600 {
		t.Fatalf("the file at its path: %q %v", b, err)
	}
	in, _ := s.Lookup(f.ID)
	if line, want := s.line(1, in), "[folder 1: "+s.Path(*f)+" (2 files, 11 B)]"; line != want {
		t.Fatalf("%q, want %q", line, want)
	}
	_, err = s.Get(f.ID, false, 0)
	wantErr(t, err, "bad_request", "id", "")
	if s.Path(*f) != s.sub(f.ID) {
		t.Fatalf("a folder is the directory named by its id: %s", s.Path(*f))
	}
	if FolderLine(2, "/d", 1, 3) != "[folder 2: /d (1 file, 3 B)]" {
		t.Fatal(FolderLine(2, "/d", 1, 3))
	}
}

// A folder needs a name; a file goes only into a folder, at a path inside it.
func TestAFolderNeedsANameAndAFileAFolderAndAPathInIt(t *testing.T) {
	s, _, q := testStore(t)
	_, err := s.Folder(" ")
	wantErr(t, err, "bad_request", "name", "")
	f, _ := s.Folder("notes")
	_, err = s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("a"), Folder: f.ID, Rel: "../a"})
	wantErr(t, err, "bad_request", "rel_path", "")
	img, err := s.Put(q, PutIn{Name: "x", Mime: "text/plain", Bytes: 1, Data: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("a"), Folder: img.Attachment.ID, Rel: "a"})
	wantErr(t, err, "bad_request", "folder", "")
}

// A path the folder holds is refused whether its file finished or is still uploading, and a full folder takes no more files.
func TestAFolderRefusesATakenPathAndAFileOverItsCap(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("notes")
	if _, err := s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 4, Data: []byte("ab"), Folder: f.ID, Rel: "a"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("x"), Folder: f.ID, Rel: "a"})
	wantErr(t, err, "bad_request", "rel_path", "")
	if _, err := s.Put(q, PutIn{Name: "c", Mime: "text/plain", Bytes: 1, Data: []byte("c"), Folder: f.ID, Rel: "c"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Put(q, PutIn{Name: "c", Mime: "text/plain", Bytes: 2, Data: []byte("c"), Folder: f.ID, Rel: "c"})
	wantErr(t, err, "bad_request", "rel_path", "") // finished: refused at the first chunk, before the rest is sent
	// A file that reached the path while this one uploaded: the upload is dropped, the file kept.
	up, _ := s.Put(q, PutIn{Name: "d", Mime: "text/plain", Bytes: 2, Data: []byte("d"), Folder: f.ID, Rel: "d"})
	os.WriteFile(filepath.Join(s.Path(*f), "d"), []byte("kept"), 0o600)
	_, err = s.Put(q, PutIn{Name: "d", Mime: "text/plain", Bytes: 2, Upload: up.Upload, Offset: 1, Data: []byte("d")})
	wantErr(t, err, "bad_request", "rel_path", "")
	if b, _ := os.ReadFile(filepath.Join(s.Path(*f), "d")); string(b) != "kept" {
		t.Fatalf("%q", b)
	}
	var im stored
	readJSON(s.sub("meta", f.ID+".json"), &im)
	im.Files = MaxFiles - 1 // with the open upload, the folder is full
	writeJSON(s.sub("meta", f.ID+".json"), im)
	_, err = s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 1, Data: []byte("x"), Folder: f.ID, Rel: "b"})
	wantErr(t, err, "refused", "", "limit")
}

// A folder's bytes count toward the unsent total, and an open folder counts as an open upload.
func TestAFolderCountsTowardTheUnsentAndOpenLimits(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("big")
	var im stored
	readJSON(s.sub("meta", f.ID+".json"), &im)
	im.Bytes = MaxUnsent
	writeJSON(s.sub("meta", f.ID+".json"), im)
	_, err := s.Put(q, PutIn{Name: "x", Mime: "text/plain", Bytes: 1, Data: []byte("x")})
	wantErr(t, err, "refused", "", "limit")
	s.remove(im.Info)
	for i := 0; i < MaxOpen; i++ {
		if _, err := s.Folder("f"); err != nil {
			t.Fatal(i, err)
		}
	}
	_, err = s.Folder("one more")
	wantErr(t, err, "refused", "", "limit")
}

// A folder no message names goes with its files after UnsentTTL, as an image does.
func TestAnUnsentFolderGoesWithItsFiles(t *testing.T) {
	s, now, q := testStore(t)
	f, _ := s.Folder("notes")
	if _, err := s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("a"), Folder: f.ID, Rel: "d/a"}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(UnsentTTL + time.Minute)
	if err := s.Sweep(q); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path(*f)); !os.IsNotExist(err) {
		t.Fatalf("the folder's directory is left: %v", err)
	}
	_, err := s.Lookup(f.ID)
	wantErr(t, err, "not_found", "", "")
}

func TestValidRel(t *testing.T) {
	for rel, want := range map[string]bool{"a.md": true, "d/e/a.md": true, "": false, "/a": false, "a/": false, "a//b": false,
		"./a": false, "a/../b": false, "..": false, `a\b`: false, "a\nb": false, strings.Repeat("a", maxPart): true,
		strings.Repeat("a", maxPart+1): false, strings.Repeat("a/", maxRel/2-1) + "ab": true, strings.Repeat("a/", maxRel/2) + "a": false} {
		if ValidRel(rel) != want {
			t.Errorf("ValidRel(%q) is %v", rel, !want)
		}
	}
}

// A path under a file the folder holds is refused before the upload; a folder already sent, a plain put typed as a folder, and a later
// chunk naming a path are refused.
func TestAFolderRefusesAPathUnderAFileASentFolderAndAFakeOne(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("notes")
	if _, err := s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("a"), Folder: f.ID, Rel: "a"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Data: []byte("b"), Folder: f.ID, Rel: "a/b"})
	wantErr(t, err, "bad_request", "rel_path", "")
	if _, err := s.Put(q, PutIn{Name: "c", Mime: "text/plain", Bytes: 1, Data: []byte("c"), Folder: f.ID, Rel: "d/c"}); err != nil {
		t.Fatalf("a directory the folder holds takes more files: %v", err)
	}
	_, err = s.Put(q, PutIn{Name: "d", Mime: "text/plain", Bytes: 2, Data: []byte("d"), Folder: f.ID, Rel: "d"})
	wantErr(t, err, "bad_request", "rel_path", "") // a directory there
	_, err = s.Put(q, PutIn{Name: "x", Mime: FolderMime, Bytes: 1, Data: []byte("x")})
	wantErr(t, err, "bad_request", "mime", "")
	if _, err := s.Claim([]string{f.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Put(q, PutIn{Name: "e", Mime: "text/plain", Bytes: 1, Data: []byte("e"), Folder: f.ID, Rel: "e"})
	wantErr(t, err, "bad_request", "folder", "")
}

// A file that cannot be placed at the end is dropped with its upload, which then holds no slot.
func TestAFileThatCannotBePlacedDropsItsUpload(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("notes")
	up, err := s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Data: []byte("b"), Folder: f.ID, Rel: "a/b"})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(s.Path(*f), "a"), []byte("a file"), 0o600) // where a directory goes
	_, err = s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Upload: up.Upload, Offset: 1, Data: []byte("b")})
	wantErr(t, err, "bad_request", "rel_path", "")
	if _, err := os.Stat(s.sub("uploads", up.Upload+".json")); !os.IsNotExist(err) {
		t.Fatalf("the upload is left: %v", err)
	}
}

// A folder that goes takes the uploads still open into it.
func TestARemovedFolderDropsItsOpenUploads(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("notes")
	up, _ := s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Data: []byte("b"), Folder: f.ID, Rel: "b"})
	other, _ := s.Put(q, PutIn{Name: "o", Mime: "text/plain", Bytes: 2, Data: []byte("o")})
	s.remove(*f)
	if _, err := os.Stat(s.sub("uploads", up.Upload+".json")); !os.IsNotExist(err) {
		t.Fatalf("the folder's upload is left: %v", err)
	}
	if _, err := os.Stat(s.sub("uploads", other.Upload+".json")); err != nil {
		t.Fatalf("another upload went: %v", err)
	}
}

// A folder with a file still uploading cannot be sent; one sent while a file uploaded takes no more.
func TestAFolderIsSentWholeAndStaysAsSent(t *testing.T) {
	s, _, q := testStore(t)
	f, _ := s.Folder("notes")
	if _, err := s.Put(q, PutIn{Name: "a", Mime: "text/plain", Bytes: 1, Data: []byte("a"), Folder: f.ID, Rel: "a"}); err != nil {
		t.Fatal(err)
	}
	other, _ := s.Folder("other")
	if _, err := s.Put(q, PutIn{Name: "o", Mime: "text/plain", Bytes: 2, Data: []byte("o"), Folder: other.ID, Rel: "o"}); err != nil {
		t.Fatal(err)
	}
	if err := s.checkID(f.ID, "attachments"); err != nil {
		t.Fatalf("another folder's upload holds this one back: %v", err)
	}
	up, _ := s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Data: []byte("b"), Folder: f.ID, Rel: "b"})
	wantErr(t, s.checkID(f.ID, "attachments"), "bad_request", "attachments", "")
	if _, err := s.Claim([]string{f.ID}); err != nil { // a send that came first
		t.Fatal(err)
	}
	_, err := s.Put(q, PutIn{Name: "b", Mime: "text/plain", Bytes: 2, Upload: up.Upload, Offset: 1, Data: []byte("b")})
	wantErr(t, err, "bad_request", "folder", "")
	if in, _ := s.Lookup(f.ID); in.Files != 1 {
		t.Fatalf("%+v", in)
	}
	if _, err := os.Stat(s.sub("uploads", up.Upload+".json")); !os.IsNotExist(err) || s.checkID(f.ID, "attachments") != nil {
		t.Fatalf("the upload is dropped and the folder may be named: %v", err)
	}
}
