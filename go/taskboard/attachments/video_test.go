package attachments

import "testing"

// A comment carries an MP4 or MOV video checked by its first bytes; an image sent with it under the same name is its poster.
func TestAVideoIsCheckedByItsBytesAndTakesItsSameNamedPictureAsPoster(t *testing.T) {
	for name, b := range map[string]string{"a.mp4": "\x00\x00\x00\x18ftypisom", "a.MOV": "\x00\x00\x00\x14ftypqt  ", "old.mov": "\x00\x00\x00\x08wide"} {
		if mime, err := FileMime(name, []byte(b)); err != nil || videos[mime] == "" {
			t.Errorf("%s: %q %v", name, mime, err)
		}
	}
	for name, b := range map[string]string{"a.mp4": "\x89PNG\r\n\x1a\n", "b.mov": "short", "c.avi": "\x00\x00\x00\x18ftypisom"} {
		if _, err := FileMime(name, []byte(b)); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	files := []Info{{ID: "v", Name: "demo.mp4", Mime: "video/mp4"}, {ID: "p", Name: "demo.png", Mime: "image/png"},
		{ID: "o", Name: "other.png", Mime: "image/png"}, {ID: "w", Name: "lone.mov", Mime: "video/quicktime"}}
	posters(files)
	if files[0].Poster != "p" || files[3].Poster != "" || files[1].Poster != "" {
		t.Fatalf("%+v", files)
	}
	s := New(t.TempDir())
	if m := s.FileMap(files[0], false); m["kind"] != "video" || m["poster"] != "p" {
		t.Fatalf("%v", m)
	}
	if m := s.FileMap(files[3], false); m["kind"] != "video" || m["poster"] != nil {
		t.Fatalf("%v", m)
	}
	if _, err := s.AddImages(nil, "", "", true, []string{"/x/demo.mp4"}); err == nil {
		t.Fatal("a message took a video")
	}
}
