package browser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// A dialog blocks its page until a DevTools session that had the page domain on when it opened answers it, and the next command's
// session is not that one. So each command answers the dialogs it sees at once, and hands the page its own alert, confirm and prompt
// for the dialogs a timer opens between commands, which answer the same way and keep what they asked for the next command to say.

type dialogSeen struct {
	Type, Message string
	Accept        bool
}

// dialogScript gives the page alert, confirm and prompt that answer as accept and reply say, and takes out what they were asked since the
// last command.
func dialogScript(accept bool, reply string) string {
	r, _ := json.Marshal(reply)
	return fmt.Sprintf(`(() => {
	const s = window[Symbol.for("hlDialogs")] ||= {asked: []};
	s.accept = %t; s.reply = %s;
	if (!s.on) {
		s.on = true;
		const ask = (type, message) => { s.asked.push({Type: type, Message: String(message ?? ""), Accept: s.accept}); return s.accept; };
		window.alert = m => { ask("alert", m); };
		window.confirm = m => ask("confirm", m);
		window.prompt = (m, d) => ask("prompt", m) ? s.reply || (d ?? "") : null;
	}
	return s.asked.splice(0);
})()`, accept, r)
}

// event answers a dialog the page opens while the command runs: a document loaded meanwhile has the browser's own.
func (p *Page) event(m message) {
	if m.Method != "Page.javascriptDialogOpening" {
		return
	}
	var d struct{ Type, Message string }
	_ = json.Unmarshal(m.Params, &d)
	accept := p.b.Accept || d.Type == "beforeunload" // a page that is left is left: the command asked for it
	p.asked(d.Type, d.Message, accept)
	answer(p.c, accept, p.b.Reply)
}

// answer sends the dialog's answer without waiting for it: the call the dialog holds is what is being read.
func answer(c *conn, accept bool, reply string) {
	c.next++
	params := map[string]any{"accept": accept}
	if reply != "" {
		params["promptText"] = reply
	}
	_ = c.ws.SetWriteDeadline(time.Now().Add(callWithin))
	_ = c.ws.WriteJSON(map[string]any{"id": c.next, "method": "Page.handleJavaScriptDialog", "params": params})
}

func (p *Page) asked(kind, msg string, accept bool) {
	how := "dismissed"
	if accept {
		how = "accepted"
	}
	p.Asked = append(p.Asked, kind+" "+strconv.Quote(msg)+" "+how)
}

// SetDialogs says how the page's dialogs are answered from now on: accepted, an accepted prompt answering reply, or dismissed.
func (b *Browser) SetDialogs(accept bool, reply string) error {
	return b.update(func(b *Browser) { b.Accept, b.Reply = accept, reply })
}
