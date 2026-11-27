package prompts

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
)

// StreamSource marks a prompt row a Chat agent's session asked on its stream: answered by writing the decision back on the stream.
const StreamSource = "stream"

// questionTool is the tool an agent asks the person a question with; its answer goes back as the tool's input.
const questionTool = "AskUserQuestion"

// Waits is whether a call waits on the row for its answer (the hook's, or a Chat session's stream), rather than a screen showing it.
func (b *Block) Waits() bool { s := b.Detail["source"]; return s == HookSource || s == StreamSource }

// AskStream is a person's answer to a Chat agent's ask to use tool, waited for with no time limit until ctx ends (ok false). A question
// is raised one prompt at a time and answered as the tool's input; any other tool is a permission prompt, as the hook raises it.
func (e *Env) AskStream(ctx context.Context, agentName, tool string, input map[string]any) (a agent.AskAnswer, ok bool, err error) {
	if tool == questionTool {
		return e.askQuestions(ctx, agentName, input)
	}
	q, err := e.permissionQuestion(agentName, StreamSource, tool, input)
	if err != nil {
		return a, false, err
	}
	d, answered, err := e.raise(ctx, q, false)
	if err != nil || !answered {
		return a, false, err
	}
	if !d.Allow {
		return agent.AskAnswer{Message: denyMessage(d.Note)}, true, nil
	}
	a = agent.AskAnswer{Allow: true}
	if r, ok := ParseRule(d.Rule); ok {
		a.Rules = []agent.Rule{{Tool: r.Tool, Content: r.Content}}
	}
	return a, true, nil
}

// askQuestions raises each of input's questions in turn and allows the tool with the answers in its input: question text to the
// picked label, labels joined by ", " for several, or the typed text. A question the person cancels denies the tool.
func (e *Env) askQuestions(ctx context.Context, agentName string, input map[string]any) (a agent.AskAnswer, ok bool, err error) {
	list, _ := input["questions"].([]any)
	answers := map[string]any{}
	asked := 0
	for _, item := range list {
		qm, _ := item.(map[string]any)
		text, _ := qm["question"].(string)
		if text == "" {
			continue
		}
		var options []any
		var labels []string
		panel := []string{text}
		opts, _ := qm["options"].([]any)
		for _, o := range opts {
			om, _ := o.(map[string]any)
			label, _ := om["label"].(string)
			options = append(options, map[string]any{"label": label, "checked": false})
			labels = append(labels, label)
			panel = append(panel, label)
		}
		multi, _ := qm["multiSelect"].(bool)
		q := question{agent: agentName, kind: "question", title: text, panel: panel, detail: map[string]any{"agent": agentName,
			"vendor": e.vendorOf(agentName), "source": StreamSource, "tool": questionTool, "title": text, "lines": panel,
			"options": options, "multi_select": multi}, labels: labels}
		d, answered, err := e.raise(ctx, q, false)
		if err != nil || !answered {
			return a, false, err
		}
		if !d.Allow {
			return agent.AskAnswer{Message: "The person did not answer the question."}, true, nil
		}
		answers[text] = d.Answer
		asked++
	}
	if asked == 0 {
		return agent.AskAnswer{Message: "The question had no text, so nobody was asked."}, true, nil
	}
	in := maps.Clone(input)
	in["answers"] = answers
	return agent.AskAnswer{Allow: true, Input: in}, true, nil
}

// denyMessage is what the model reads for a person's no, with their reason.
func denyMessage(note string) string {
	if note == "" {
		return "The founder said no."
	}
	return "The founder said no: " + note
}

// questionChoice is a question row's answer for choice: an option's key, several keys on a multi-select, or {text} typed; esc cancels.
// The labels are the waiting call's own, not the row's.
func (e *Env) questionChoice(b *Block, choice any) (Choice, error) {
	opts := e.hooks.optionLabels(b.ID)
	label := func(key string) (string, bool) {
		var i int
		if _, err := fmt.Sscan(key, &i); err != nil || fmt.Sprint(i) != key || i < 1 || i > len(opts) {
			return "", false
		}
		return opts[i-1], true
	}
	offers := fmt.Sprintf("an option key 1 to %d, esc, or {text}", len(opts))
	switch c := choice.(type) {
	case string:
		if c == "esc" {
			return Choice{}, nil
		}
		if l, ok := label(c); ok {
			return Choice{Yes: true, Answer: l}, nil
		}
	case []any:
		if b.Detail["multi_select"] != true || len(c) == 0 {
			break
		}
		var picked []string
		for _, k := range c {
			l, ok := label(k.(string))
			if !ok {
				return Choice{}, bad("this prompt offers "+offers+", not "+quote(k.(string)), "choice")
			}
			if !slices.Contains(picked, l) {
				picked = append(picked, l)
			}
		}
		return Choice{Yes: true, Answer: strings.Join(picked, ", ")}, nil
	case map[string]any:
		if text, _ := c["text"].(string); strings.TrimSpace(text) != "" {
			return Choice{Yes: true, Answer: strings.TrimSpace(text)}, nil
		}
	}
	return Choice{}, bad("this prompt offers "+offers+", not "+quote(fmt.Sprint(choice)), "choice")
}
