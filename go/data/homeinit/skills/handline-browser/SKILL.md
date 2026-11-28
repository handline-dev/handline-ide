---
name: handline-browser
description: Drive a web browser with hl browser - open a page, read its text or accessibility tree, click, type, press keys, hover, drag, select, scroll, go back, use tabs, upload and download files, answer dialogs, read the console and network, run a script, save a screenshot. Use whenever you need to look at, test or take a picture of a web page or web app, local or on the internet.
---

# hl browser

Your own headless Chrome: no other agent shares it, it keeps its pages between commands, and it closes when your session ends. A target is a ref from `read --tree` (`e12`) or a CSS selector. Each verb below follows `hl browser`; `hl browser` alone lists them all.

- `open <url>` (`--show` shows the window); `read --tree` lists elements with refs (read again after a load); plain `read` is the text.
- `click e12`, `type e7 "hello" --enter`, `press Shift+Tab`, `hover e3`, `drag e3 e9`, `select e5 Green`.
- `scroll down [600] [--in e4]`; `back`, `forward`, `reload`; `viewport 390 844`.
- `wait --text "Saved"` or `wait '#result'` (10 s unless `--timeout 30s`).
- `tab` lists tabs; `tab new <url>`, `tab 2`, `tab close 2`.
- `list` shows what is open, its tabs and who has it or watches it; it acts on nothing, so it works while a person has the page.
- `upload e8 file.pdf` and `download e6 [dir]` use paths inside your worktree.
- A dialog the page opens is answered at once and printed; `dialog accept [reply]` or `dialog dismiss` (the default) sets the answer.
- `console` shows the page's logs and errors; `network` what it loaded, with each status.
- `eval "document.title"` runs a script; `shot out.png [--full]` saves a PNG to attach.
- `close`.
