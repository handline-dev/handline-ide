package prompts

import "fmt"

// Choice is what Answer hands a hook's question: Yes allows it, otherwise it is denied, with Note as the reason.
type Choice struct {
	Yes  bool
	Note string
}

// Answer answers the question of block blockID with choice. Only the hook's questions are answered here: they are the only prompt rows
// there are, and each is answered by returning the decision to the call that waits on it.
func (e *Env) Answer(blockID int64, c Choice) (bool, string) {
	row, err := e.get(blockID)
	if err != nil {
		return false, err.Error()
	}
	if row == nil {
		return false, fmt.Sprintf("no prompt %d", blockID)
	}
	if row.GoneAt.Valid {
		return e.record(row, false, row.Agent+": the prompt is gone - nothing sent")
	}
	if !row.IsHook() {
		return e.record(row, false, row.Agent+": this prompt was not asked through the hook - answer it in its terminal")
	}
	return e.answerHook(row, c)
}

func (e *Env) record(row *Block, ok bool, said string) (bool, string) {
	okInt := 0
	if ok {
		okInt = 1
	}
	e.DB.Exec("UPDATE prompt_blocks SET answered_at = ?, answer_ok = ?, answer_said = ? WHERE id = ?", e.now().Unix(), okInt, said, row.ID)
	return ok, said
}
