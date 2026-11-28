package browser

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type keyDef struct {
	code string
	vk   int
	text string
}

// keys are the named keys press knows; any one character is a key too.
var keys = map[string]keyDef{
	"Enter": {"Enter", 13, "\r"}, "Tab": {"Tab", 9, ""}, "Escape": {"Escape", 27, ""}, "Backspace": {"Backspace", 8, ""},
	"Delete": {"Delete", 46, ""}, "Insert": {"Insert", 45, ""}, "Space": {"Space", 32, " "},
	"ArrowLeft": {"ArrowLeft", 37, ""}, "ArrowUp": {"ArrowUp", 38, ""}, "ArrowRight": {"ArrowRight", 39, ""},
	"ArrowDown": {"ArrowDown", 40, ""}, "Home": {"Home", 36, ""}, "End": {"End", 35, ""}, "PageUp": {"PageUp", 33, ""},
	"PageDown": {"PageDown", 34, ""},
}

func init() {
	for i := 1; i <= 12; i++ {
		f := fmt.Sprintf("F%d", i)
		keys[f] = keyDef{f, 111 + i, ""}
	}
}

// modifiers are the bits CDP gives each modifier, under the names a key combination may use.
var modifiers = map[string]int{"Alt": 1, "Option": 1, "Control": 2, "Ctrl": 2, "Meta": 4, "Cmd": 4, "Command": 4, "Shift": 8}

// editing are the commands a Meta combination runs: on macOS the browser's own shortcuts are not keys the page gets.
var editing = map[string]string{"a": "selectAll", "c": "copy", "x": "cut", "v": "paste", "z": "undo"}

type keyPress struct {
	key, code, text string
	vk, mods        int
	commands        []string
}

// parseKey reads a key combination such as Enter, Shift+Tab, Control+a or Meta+Shift+z.
func parseKey(combo string) (keyPress, error) {
	parts := strings.Split(combo, "+")
	name := parts[len(parts)-1]
	if name == "" && strings.HasSuffix(combo, "+") { // the + key itself
		name, parts = "+", parts[:len(parts)-1]
	}
	k := keyPress{}
	for _, m := range parts[:len(parts)-1] {
		bit, ok := modifiers[m]
		if !ok {
			return k, fmt.Errorf("%s is not a modifier: Alt, Control, Meta or Shift", m)
		}
		k.mods |= bit
	}
	if d, ok := keys[name]; ok {
		k.key, k.code, k.vk, k.text = name, d.code, d.vk, d.text
		if name == "Space" {
			k.key = " "
		}
	} else if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		k.key, k.text = name, name
		if k.mods&8 != 0 {
			k.key, k.text = strings.ToUpper(name), strings.ToUpper(name)
		}
		switch up := strings.ToUpper(name); {
		case up >= "A" && up <= "Z" && len(up) == 1:
			k.code, k.vk = "Key"+up, int(up[0])
		case r >= '0' && r <= '9':
			k.code, k.vk = "Digit"+name, int(r)
		}
	} else {
		return k, fmt.Errorf("%s is not a key: a character, or Enter, Tab, Escape, Backspace, Delete, Space, an arrow (ArrowDown), "+
			"Home, End, PageUp, PageDown or F1 to F12", name)
	}
	if k.mods&(1|2|4) != 0 {
		k.text = ""
	}
	if c, ok := editing[strings.ToLower(name)]; ok && k.mods == 4 {
		k.commands = []string{c}
	} else if strings.ToLower(name) == "z" && k.mods == 4|8 {
		k.commands = []string{"redo"}
	}
	return k, nil
}

// Press presses and lets go of a key combination in the focused element.
func (p *Page) Press(combo string) error {
	k, err := parseKey(combo)
	if err != nil {
		return err
	}
	down := map[string]any{"type": "rawKeyDown", "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "modifiers": k.mods}
	if k.text != "" {
		down["type"], down["text"] = "keyDown", k.text
	}
	if k.commands != nil {
		down["commands"] = k.commands
	}
	if _, err := p.c.call("Input.dispatchKeyEvent", down); err != nil {
		return err
	}
	_, err = p.c.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k.key, "code": k.code,
		"windowsVirtualKeyCode": k.vk, "modifiers": k.mods})
	return err
}
