package layers

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var deliveryTest = regexp.MustCompile(`\b(delivered|typed|landed|read)_at IS (NOT )?NULL`)

// Outside go/data/db no SQL statement tests two of the delivery columns together: a combination has a name in delivery.go.
// A statement is a line and the lines it continues with a trailing +.
func TestNoDeliveryColumnsAreCombinedOutsideDB(t *testing.T) {
	root, _ := filepath.Abs("../..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join("data", "db") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		for i := 0; i < len(lines); i++ {
			start, stmt := i, lines[i]
			for strings.HasSuffix(strings.TrimSpace(lines[i]), "+") && i+1 < len(lines) {
				i++
				stmt += lines[i]
			}
			cols := map[string]bool{}
			for _, m := range deliveryTest.FindAllStringSubmatch(stmt, -1) {
				cols[m[1]] = true
			}
			if len(cols) > 1 {
				t.Errorf("%s:%d tests %d delivery columns together; use a predicate of db/delivery.go: %s", rel, start+1, len(cols), strings.TrimSpace(stmt))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
