package nodes

import (
	"reflect"
	"testing"
)

// A path under the master's home is the same path under the runner's, in every text of an answer; a path that only starts like the
// home, or holds it inside, is left alone. The attachments a text names are found, a sentence's last stop not taken for part of the path.
func TestPathsUnderTheMastersHomeAreTheRunnersAndAttachmentsAreFound(t *testing.T) {
	got := swapAll(map[string]any{"a": "/m/home/agents/k/memory and /m/homework", "b": []any{"/m/home", 3}}, "/m/home", "/r/h")
	want := map[string]any{"a": "/r/h/agents/k/memory and /m/homework", "b": []any{"/r/h", 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	r := &Registry{Home: "/m/home"}
	refs := r.attachmentRefs("see /m/home/attachments/ab12.png. and `/m/home/attachments/cd.pdf`, not /m/home/shared/x")
	if !reflect.DeepEqual(refs, []string{"/m/home/attachments/ab12.png", "/m/home/attachments/cd.pdf"}) {
		t.Fatalf("%q", refs)
	}
	if refs := r.attachmentRefs("/m/home/attachments/../accounts and /m/home/attachments/x/../../db.sqlite"); len(refs) != 0 {
		t.Fatalf("a path out of the attachments was taken for one: %q", refs)
	}
	if got := swap("in /m/home. and /x/m/home/y", "/m/home", "/r/h"); got != "in /r/h. and /x/m/home/y" {
		t.Fatalf("%q", got)
	}
}
