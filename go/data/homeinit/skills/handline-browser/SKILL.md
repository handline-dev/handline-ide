---
name: handline-browser
description: Drive a web browser with hl browser - open a page, read its text or accessibility tree, click, type, wait, run a script, save a screenshot. Use whenever you need to look at, test or take a picture of a web page or web app, local or on the internet.
---

# hl browser

Your own headless Chrome: no other agent shares it, it keeps its one page between commands, and it closes when your session ends. Nothing to install.

- `hl browser open <url>` loads the page; `--show` shows the window.
- `hl browser read --tree` lists the elements, each with a ref (`[e12]`); plain `read` is the visible text. Read the tree again after the page loads anew.
- `hl browser click e12`, `hl browser type e7 "hello" --enter`: a ref or a CSS selector.
- `hl browser wait --text "Saved"` or `wait '#result'` (10 s unless `--timeout 30s`).
- `hl browser eval "document.title"` runs a script in the page.
- `hl browser shot out.png [--full]` saves a PNG; attach it to show someone the page.
- `hl browser close`.
