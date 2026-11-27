package watch

import (
	"fmt"
	"testing"
)

// The read as GitHub answered it on a real pull request: a comment, a submitted review with words and a line comment, a pending review
// with one, and a bot's comment and an outdated line comment added. The pending review's comment waits for its submit; a review with no
// words is left to its verdict; an outdated line comment keeps the line it was written on; a line comment is said at its review's submit.
func TestParseCommentsTakesWhatPeopleSubmitted(t *testing.T) {
	out := []byte(`{"data":{"repository":{"pullRequest":{
"comments":{"nodes":[{"databaseId":7211539098,"author":{"__typename":"User"},"authorAssociation":"MEMBER","body":"probe: a general comment","createdAt":"2026-11-27T06:27:06Z"},
 {"databaseId":7211539100,"author":{"__typename":"Bot"},"authorAssociation":"NONE","body":"preview ready","createdAt":"2026-11-27T06:27:09Z"}]},
"reviews":{"nodes":[
 {"databaseId":6084357915,"state":"PENDING","body":"","submittedAt":null,"author":{"__typename":"User"},
  "comments":{"nodes":[{"databaseId":4716333592,"path":"probe-status-bcf790.txt","line":1,"originalLine":1,"body":"probe: a pending line comment","createdAt":"2026-11-27T06:27:08Z"}]}},
 {"databaseId":6084357770,"state":"COMMENTED","body":"probe: a review body","submittedAt":"2026-11-27T06:27:07Z","author":{"__typename":"User"},"authorAssociation":"MEMBER",
  "comments":{"nodes":[{"databaseId":4716333476,"path":"probe-status-bcf790.txt","line":1,"originalLine":1,"body":"probe: a line comment","createdAt":"2026-11-27T06:27:07Z"},
   {"databaseId":4716333477,"path":"a.go","line":null,"originalLine":9,"body":"outdated","createdAt":"2026-11-27T06:27:10Z"}]}},
 {"databaseId":6084357999,"state":"APPROVED","body":"","submittedAt":"2026-11-27T06:28:00Z","author":{"__typename":"User"},"comments":{"nodes":[]}}]}}}}}`)
	cs, err := parseComments(out)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cs {
		got = append(got, fmt.Sprintf("%s %s %s %s:%d %q %v %v %s", c.Key, c.Kind, c.Review, c.Path, c.Line, c.Body, c.Bot, c.Member, c.At.Format("15:04:05")))
	}
	want := []string{
		`c7211539098 comment  :0 "probe: a general comment" false true 06:27:06`,
		`r6084357770 review COMMENTED :0 "probe: a review body" false true 06:27:07`,
		`l4716333476 line COMMENTED probe-status-bcf790.txt:1 "probe: a line comment" false true 06:27:07`,
		`l4716333477 line COMMENTED a.go:9 "outdated" false true 06:27:07`,
		`c7211539100 comment  :0 "preview ready" true false 06:27:09`,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	if _, err := parseComments([]byte(`{"data":{"repository":{"pullRequest":null}}}`)); err == nil {
		t.Fatal("a missing pull request reads")
	}
}
