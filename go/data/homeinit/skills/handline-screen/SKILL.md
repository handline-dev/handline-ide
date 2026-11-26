---
name: handline-screen
description: Capture a native app window with hl screen when you need to see it. Use hl browser for a web page and hl sim for a phone app first; never run screencapture from the shell.
---

# hl screen

Use `hl browser shot` for a web page and `hl sim screenshot` for a phone app. `node tools/app.mjs shot` pictures Handline's own test app. Only a native window needs this.

1. `hl screen list` prints each on-screen window: id, app, title.
2. `hl screen shot --app <name>` saves that app's front window as a PNG and prints its path (`--window <id>` for a given one). Look at the file.

Handline takes the picture, records it, and macOS asks for screen recording once, for Handline. Never run `screencapture` yourself: it makes macOS ask in the name of your terminal. A refusal names the setting that blocks it, or says where to turn Handline on.
