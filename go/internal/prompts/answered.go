package prompts

import "github.com/handline/handline/go/protocol/rpc"

// recordAnswer keeps what prompt.answer answered on its block, beside the answered_at record() wrote, so the chat's prompt item says it
// once the prompt closes: Allowed or Denied, a deny's reason as the note, and who answered.
func (e *Env) recordAnswer(c *rpc.Conn, b *Block, pick Choice) error {
	label, note := "Denied", pick.Note
	if pick.Yes {
		label, note = "Allowed", ""
	}
	by, _ := c.Caller.Info["name"].(string)
	_, err := e.DB.Exec("UPDATE prompt_blocks SET answer_label = ?, answer_note = ?, answer_by = ? WHERE id = ?", label, note, by, b.ID)
	return err
}
