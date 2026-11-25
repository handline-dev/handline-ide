package rulesfold

import (
	"strings"

	"github.com/handline/handline/go/data/db"
)

// apply writes res.Rules over person's kept rules: the dropped ones are removed, a changed or merged one replaced, a new one added.
func apply(q db.Q, person string, res Result) error {
	for _, r := range res.Dropped {
		for _, id := range r.from {
			if _, err := db.RemoveLesson(q, id, By); err != nil {
				return err
			}
		}
	}
	var revs []db.Revision
	for _, r := range res.Rules {
		if ref, ok := strings.CutPrefix(r.QuoteRef, db.MsgPrefix); ok && len(ref) > db.MsgMinPrefix {
			r.QuoteRef = db.MsgNameOf(q, ref)
		}
		if r.stored != "" {
			continue
		}
		revs = append(revs, db.Revision{IDs: r.from, Text: r.Line(), Task: r.Task})
	}
	return db.ReviseLessons(q, person, revs, By)
}

// same is whether a rule came back as it was kept: then its lesson is left as it is, in whatever form a person wrote it.
func same(a, b Rule) bool {
	return a.Area == b.Area && a.Text == b.Text && a.Count == b.Count && a.Quote == b.Quote && a.QuoteRef == b.QuoteRef
}
