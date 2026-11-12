package homeinit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home := filepath.Join(t.TempDir(), "home")
	var said []string
	say := func(f string, a ...any) { said = append(said, f) }
	if err := Setup(home, "hl", say); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"agents", "accounts", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if len(said) == 0 {
		t.Fatal("a fresh home said nothing")
	}
	said = nil
	if err := Setup(home, "hl", say); err != nil {
		t.Fatal(err)
	}
	if len(said) != 0 {
		t.Errorf("a home already set up said %v", said)
	}
}
