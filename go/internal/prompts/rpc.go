package prompts

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// prompt.answer: the founder's allow or deny to a permission question an agent's hook is waiting on. prompt.list is the reads'. An
// answer names the prompt by its id, the block's fingerprint. When the open prompt is another, nothing is done and the reply is
// stale_prompt with the current one.

// Register adds prompt.answer.
func Register(s *rpc.Server, e *Env) {
	s.Register("prompt.answer", rpc.Method{Handler: e.answer})
	s.Register("permission.ask", rpc.Method{Waits: true, Handler: e.permissionAsk})
}

func bad(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

func (e *Env) agentNamed(params map[string]any) (string, error) {
	name, _ := params["agent"].(string)
	if name == "" {
		return "", bad("agent is a name", "agent")
	}
	for _, n := range paths.ListAgents(e.Home) {
		if n == name {
			return name, nil
		}
	}
	err := rpc.NotFoundError("no such agent: " + name)
	err.Data = map[string]any{"what": "agent", "name": name}
	return "", err
}

// answerFor is what Answer takes for choice on this block: allow or deny, the two a hook's question offers; bad_request for anything else.
func answerFor(b *Block, choice any) (Choice, error) {
	c, _ := choice.(string)
	if !b.IsHook() || (c != "allow" && c != "deny") {
		return Choice{}, bad(fmt.Sprintf("this prompt offers allow, deny, not %s", pyRepr(fmt.Sprint(choice))), "choice")
	}
	return Choice{Yes: c == "allow"}, nil
}

func pyRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	s = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s)
	if q == "'" {
		s = strings.ReplaceAll(s, "'", `\'`)
	}
	return q + s + q
}

func (e *Env) answer(c *rpc.Conn, params map[string]any) (any, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return nil, bad("prompt is the id of the prompt the caller saw", "prompt")
	}
	choice := params["choice"]
	switch ch := choice.(type) {
	case []any:
		for _, k := range ch {
			if _, ok := k.(string); !ok {
				return nil, bad("choice is an array of option keys", "choice")
			}
		}
	case string, map[string]any:
	default:
		return nil, bad("choice is an option key, or {text} with the text", "choice")
	}
	name, err := e.agentNamed(params)
	if err != nil {
		return nil, err
	}
	block, err := e.OpenBlockOf(name)
	if err != nil {
		return nil, err
	}
	if block == nil || block.Fingerprint != prompt {
		current, err := e.Reads.OpenPrompt(name)
		if err != nil {
			return nil, err
		}
		return nil, StaleError(current)
	}
	pick, err := answerFor(block, choice)
	if err != nil {
		return nil, err
	}
	if note, has := params["note"]; has {
		text, ok := note.(string)
		if !ok || !block.IsHook() || pick.Yes {
			return nil, bad("note is a deny's reason, for a question from the hook", "note")
		}
		pick.Note = strings.TrimSpace(text)
	}
	ok, said := e.Answer(block.ID, pick)
	if !ok {
		return nil, rpc.Refused(said, "", "")
	}
	if err := e.recordAnswer(c, block, pick); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true, "said": said}, nil
}
