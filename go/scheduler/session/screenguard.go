package session

import (
	"regexp"
)

// screencapture as a command word: first in the command or after a separator, with or without its path.
var screencapturePat = regexp.MustCompile("(^|[;&|(){`'\"=\\s])(/[^\\s'\"]*/)?screencapture(\\s|['\"`;&|)]|$)")

// RunsScreencapture is whether a Bash command runs the OS screenshot command.
func RunsScreencapture(tool string, toolInput any) bool {
	in, _ := toolInput.(map[string]any)
	command, _ := in["command"].(string)
	return tool == "Bash" && screencapturePat.MatchString(command)
}

// ScreencaptureWhy is the refusal of a shell screencapture while Send screenshot commands through Handline is on: it names what to use.
const ScreencaptureWhy = "screencapture from the shell makes macOS ask for screen recording in the name of the terminal. Use hl screen shot --app <name> for a native window, hl browser shot <file> for a web page, hl sim screenshot <file> for a phone app, node tools/app.mjs shot for Handline's own test app."
