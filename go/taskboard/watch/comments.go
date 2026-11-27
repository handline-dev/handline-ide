package watch

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Comment is one thing a person wrote on a pull request: a comment on it (Path ""), a review's own words, or a line comment of a review.
// Key names it once among them all: GitHub numbers each kind on its own. Who wrote it is not taken: Bot says only whether a bot did, and
// Member whether its writer belongs to the repository (its owner, a member of its organization, or a collaborator).
type Comment struct {
	Key    string
	Kind   string // CommentPlain, CommentReview or CommentLine
	Review string // a review's state as GitHub says it (COMMENTED, APPROVED, CHANGES_REQUESTED), for a review and its line comments
	Path   string
	Line   int // the line in the pull request's newest diff, else the line it was written on; 0 for none
	Body   string
	At     time.Time
	Bot    bool
	Member bool
}

// What a Comment is.
const (
	CommentPlain, CommentReview, CommentLine = "comment", "review", "line"
)

// commentsQuery reads a pull request's comments and its reviews with their line comments in one call. A review still pending (written,
// not submitted) is seen only by its writer's login, with its comments already numbered: those wait for the submit (Comments).
const commentsQuery = `query($o:String!,$n:String!,$p:Int!){repository(owner:$o,name:$n){pullRequest(number:$p){` +
	`comments(last:100){nodes{databaseId author{__typename} authorAssociation body createdAt}} ` +
	`reviews(last:100){nodes{databaseId state body submittedAt author{__typename} authorAssociation ` +
	`comments(first:100){nodes{databaseId path line originalLine body}}}}}}}`

type ghAuthor struct {
	Typename string `json:"__typename"`
}

type ghComments struct {
	Data struct {
		Repository struct {
			PullRequest *struct {
				Comments struct {
					Nodes []struct {
						DatabaseID  int64 `json:"databaseId"`
						Author      *ghAuthor
						Association string `json:"authorAssociation"`
						Body        string
						CreatedAt   time.Time
					}
				}
				Reviews struct {
					Nodes []struct {
						DatabaseID  int64 `json:"databaseId"`
						State       string
						Body        string
						SubmittedAt *time.Time
						Author      *ghAuthor
						Association string `json:"authorAssociation"`
						Comments    struct {
							Nodes []struct {
								DatabaseID   int64 `json:"databaseId"`
								Path         string
								Line         *int
								OriginalLine *int
								Body         string
							}
						}
					}
				}
			}
		}
	}
}

// Comments are what people wrote on pull request number of repo (owner/name), read now through this machine's gh login: its comments,
// each submitted review's words when it has any, and their line comments, oldest first. A pending review and its comments are left out
// until it is submitted; a review with no words of its own is only its verdict, which the gate reads.
func (g *GitHub) Comments(ctx context.Context, repo string, number int64) ([]Comment, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || !RepoName(repo) {
		return nil, fmt.Errorf("%q is no owner/name", repo)
	}
	out, err := g.gh(ctx, "api", "graphql", "-f", "query="+commentsQuery, "-f", "o="+owner, "-f", "n="+name, "-F", "p="+strconv.FormatInt(number, 10))
	if err != nil {
		if gone(err) {
			return nil, goneError{err.Error()}
		}
		return nil, err
	}
	return parseComments(out)
}

func parseComments(out []byte) ([]Comment, error) {
	var read ghComments
	if err := json.Unmarshal(out, &read); err != nil {
		return nil, err
	}
	pr := read.Data.Repository.PullRequest
	if pr == nil {
		return nil, goneError{"no such pull request"}
	}
	bot := func(a *ghAuthor) bool { return a != nil && a.Typename == "Bot" }
	var cs []Comment
	for _, c := range pr.Comments.Nodes {
		cs = append(cs, Comment{Key: "c" + strconv.FormatInt(c.DatabaseID, 10), Kind: CommentPlain, Body: c.Body, At: c.CreatedAt, Bot: bot(c.Author),
			Member: member(c.Association)})
	}
	for _, r := range pr.Reviews.Nodes {
		if r.State == "PENDING" || r.SubmittedAt == nil {
			continue
		}
		if strings.TrimSpace(r.Body) != "" {
			cs = append(cs, Comment{Key: "r" + strconv.FormatInt(r.DatabaseID, 10), Kind: CommentReview, Review: r.State, Body: r.Body,
				At: *r.SubmittedAt, Bot: bot(r.Author), Member: member(r.Association)})
		}
		for _, l := range r.Comments.Nodes {
			line := 0
			switch {
			case l.Line != nil:
				line = *l.Line
			case l.OriginalLine != nil:
				line = *l.OriginalLine
			}
			// a line comment is said when its review is submitted, whenever it was drafted
			cs = append(cs, Comment{Key: "l" + strconv.FormatInt(l.DatabaseID, 10), Kind: CommentLine, Review: r.State, Path: l.Path, Line: line,
				Body: l.Body, At: *r.SubmittedAt, Bot: bot(r.Author), Member: member(r.Association)})
		}
	}
	sortComments(cs)
	return cs, nil
}

// member is whether GitHub's author association says the writer belongs to the repository.
func member(association string) bool {
	switch association {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	}
	return false
}

// sortComments orders cs oldest first, a review's words before its line comments when they share a time.
func sortComments(cs []Comment) {
	rank := map[string]int{CommentPlain: 0, CommentReview: 1, CommentLine: 2}
	slices.SortStableFunc(cs, func(a, b Comment) int { return cmp.Or(a.At.Compare(b.At), cmp.Compare(rank[a.Kind], rank[b.Kind])) })
}
