package layers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// clocked are the packages the account scenarios run on a fake clock (docs/design/account-assignment-scenarios.md): each reads the
// time through an injected Now or db.Now, and the wall clock only as a Now's default, in a function named now.
var clocked = []string{"accounts/usage", "scheduler/lifecycle"}

// wallClock are the time functions that read the wall clock.
var wallClock = map[string]bool{"Now": true, "Since": true, "Until": true}

func TestTheScenarioPackagesReadTheTimeOnlyThroughTheirNow(t *testing.T) {
	t.Parallel()
	var bad []string
	for _, pkg := range clocked {
		files, err := filepath.Glob(filepath.Join("..", "..", pkg, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no files in %s: %v", pkg, err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range file.Decls {
				where := "a package declaration"
				if fn, ok := d.(*ast.FuncDecl); ok {
					if where = fn.Name.Name; where == "now" {
						continue
					}
				}
				ast.Inspect(d, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok && wallClock[sel.Sel.Name] {
						if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" {
							bad = append(bad, pkg+"/"+filepath.Base(f)+": "+where)
						}
					}
					return true
				})
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("the wall clock read outside an injected clock's default:\n%s", strings.Join(bad, "\n"))
	}
}
