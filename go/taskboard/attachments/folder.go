package attachments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/kit/textsafe"
)

// A folder (contract x-attachments, folders) is one attachment that holds files: a directory named by its id, each file at the
// rel_path it was put with. Its meta keeps the sum of their bytes and their count.

const (
	FolderMime = "inode/directory"
	MaxFiles   = 2000 // files in one folder
	maxRel     = 1024 // bytes of a rel_path
	maxPart    = 255  // bytes of one name in it, as a disk takes it
)

// IsFolder is whether i is a folder.
func (i Info) IsFolder() bool { return i.Mime == FolderMime }

// Folder opens an empty folder named name.
func (s *Store) Folder(name string) (*Info, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	name = FileName(name)
	if name == "" {
		return nil, badParam("name is the folder's name", "name")
	}
	if open := s.openCount(); open >= MaxOpen {
		return nil, limit(fmt.Sprintf("%d uploads are open already: send or wait for them", MaxOpen))
	}
	im := stored{Info: Info{ID: newID(), Name: name, Mime: FolderMime}, Created: s.now().Unix()}
	if err := os.Mkdir(s.Path(im.Info), 0o700); err != nil {
		return nil, err
	}
	if err := writeJSON(s.sub("meta", im.ID+".json"), im); err != nil {
		os.Remove(s.Path(im.Info))
		return nil, err
	}
	return &im.Info, nil
}

// openCount is the uploads open and the folders no message has named yet.
func (s *Store) openCount() int {
	n := len(s.glob("uploads", "*.json"))
	for _, p := range s.glob("meta", "*.json") {
		var im stored
		if readJSON(p, &im) == nil && im.IsFolder() && im.Claimed == 0 {
			n++
		}
	}
	return n
}

// ValidRel is whether rel is a path inside a folder: parts between `/`, none empty, `.` or `..`, and no leading `/`.
func ValidRel(rel string) bool {
	if len(rel) > maxRel || strings.ContainsAny(rel, "\\\x00") || textsafe.Oneline(rel) != rel {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || len(part) > maxPart {
			return false
		}
	}
	return true
}

// intoFolder checks a first chunk that goes into folder at rel: the folder is kept, has room, and holds nothing at rel yet, whether
// finished or still uploading.
func (s *Store) intoFolder(folder, rel string) error {
	if !ValidRel(rel) {
		return badParam("rel_path is a path inside the folder: no empty, . or .. part, no leading /", "rel_path")
	}
	var im stored
	if _, err := s.Lookup(folder); err != nil {
		return err
	}
	if err := readJSON(s.sub("meta", folder+".json"), &im); err != nil || !im.IsFolder() {
		return badParam("folder is an id from attachment.folder", "folder")
	}
	if im.Claimed != 0 {
		return badParam("the folder was sent already: open another", "folder")
	}
	files := im.Files
	for _, p := range s.glob("uploads", "*.json") {
		var u upload
		if readJSON(p, &u) != nil || u.Folder != folder {
			continue
		}
		if u.Rel == rel {
			return badParam("the folder already has a file at "+rel, "rel_path")
		}
		files++
	}
	if files >= MaxFiles {
		return limit(fmt.Sprintf("a folder holds at most %d files", MaxFiles))
	}
	dir := s.Path(im.Info)
	parts := strings.Split(rel, "/")
	for i := range parts {
		st, err := os.Lstat(filepath.Join(dir, filepath.Join(parts[:i+1]...)))
		if err != nil {
			break
		}
		if i == len(parts)-1 || !st.IsDir() {
			return badParam("the folder already has a file at "+strings.Join(parts[:i+1], "/"), "rel_path")
		}
	}
	return nil
}

// uploadingInto is whether a file is still uploading into folder.
func (s *Store) uploadingInto(folder string) bool {
	for _, p := range s.glob("uploads", "*.json") {
		var u upload
		if readJSON(p, &u) == nil && u.Folder == folder {
			return true
		}
	}
	return false
}

// finishInto moves a whole upload to its place in its folder and counts it there; the reply's attachment is the folder as it is now.
func (s *Store) finishInto(uploadID string, up upload) (*Info, error) {
	var im stored
	if err := readJSON(s.sub("meta", up.Folder+".json"), &im); err != nil || !im.IsFolder() {
		s.drop(uploadID)
		return nil, unknownImage(up.Folder)
	}
	if im.Claimed != 0 { // sent while this file uploaded: what the agent was told stands
		s.drop(uploadID)
		return nil, badParam("the folder was sent already: open another", "folder")
	}
	dst := filepath.Join(s.Path(im.Info), filepath.FromSlash(up.Rel))
	if _, err := os.Lstat(dst); err == nil || os.MkdirAll(filepath.Dir(dst), 0o700) != nil {
		s.drop(uploadID)
		return nil, badParam("the folder already has a file at or above "+up.Rel, "rel_path")
	}
	if err := os.Rename(s.sub("uploads", uploadID+".part"), dst); err != nil {
		s.drop(uploadID)
		return nil, err
	}
	im.Bytes += up.Bytes
	im.Files++
	if err := writeJSON(s.sub("meta", im.ID+".json"), im); err != nil {
		return nil, err
	}
	os.Remove(s.sub("uploads", uploadID+".json"))
	return &im.Info, nil
}

// FolderLine is how an agent is told of folder n: `[folder N: /path (12 files, 3 MB)]`.
func FolderLine(n int, path string, files int, bytes int64) string {
	word := "files"
	if files == 1 {
		word = "file"
	}
	return fmt.Sprintf("[folder %d: %s (%d %s, %s)]", n, path, files, word, size(bytes))
}
