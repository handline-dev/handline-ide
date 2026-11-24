package homeinit

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Handline's own skills: each folder under skills/ ships in the build and is written to the skills folder every account sees. A skill
// Handline owns is named SkillPrefix+..., holds a SkillMark file with the hash of what was written, and is the only kind it replaces or
// removes: a folder without the mark is the user's, whatever its name.
//
//go:embed skills
var skillFiles embed.FS

const (
	SkillPrefix = "handline-"
	SkillMark   = ".handline-skill"
)

// InstallSkills writes Handline's skills under dir and says which it wrote or removed: one whose files changed with this build is
// replaced, one this build no longer has is removed.
func InstallSkills(dir string) ([]string, error) {
	var changed []string
	have := map[string]bool{}
	names, err := fs.ReadDir(skillFiles, "skills")
	if err != nil {
		return nil, err
	}
	for _, e := range names {
		name := e.Name()
		have[name] = true
		sum, err := skillHash(name)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(dir, name)
		if mark, owned := skillMark(target); owned && mark == sum {
			continue
		} else if !owned && lexists(target) {
			continue // the user's own skill under our name
		}
		if err := writeSkill(dir, name, sum); err != nil {
			return nil, err
		}
		changed = append(changed, name)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if _, owned := skillMark(filepath.Join(dir, e.Name())); owned && !have[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return nil, err
			}
			changed = append(changed, "-"+e.Name())
		}
	}
	return changed, nil
}

// skillMark is the hash in dir's mark and whether dir is a skill folder Handline wrote: a real folder with a mark, never a link.
func skillMark(dir string) (string, bool) {
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() || !strings.HasPrefix(filepath.Base(dir), SkillPrefix) {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(dir, SkillMark))
	return strings.TrimSpace(string(b)), err == nil
}

func lexists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// skillHash is the hash of every file name and content of the skill name in the build.
func skillHash(name string) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(skillFiles, path.Join("skills", name), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skillFiles.ReadFile(p)
		h.Write([]byte(p + "\x00"))
		h.Write(b)
		h.Write([]byte{0})
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// writeSkill puts skill name in place whole: built beside its place, then swapped in, so a stop half way leaves the old skill or none.
func writeSkill(dir, name, sum string) error {
	fresh := filepath.Join(dir, "."+name+".new")
	_ = os.RemoveAll(fresh)
	var rels []string
	err := fs.WalkDir(skillFiles, path.Join("skills", name), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rels = append(rels, p)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(rels)
	for _, p := range rels {
		b, err := skillFiles.ReadFile(p)
		if err != nil {
			return err
		}
		out := filepath.Join(fresh, filepath.FromSlash(strings.TrimPrefix(p, "skills/"+name+"/")))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(fresh, SkillMark), []byte(sum+"\n"), 0o644); err != nil {
		return err
	}
	target := filepath.Join(dir, name)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(fresh, target)
}
