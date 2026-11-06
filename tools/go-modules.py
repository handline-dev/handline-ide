#!/usr/bin/env python3
"""The Go modules a change needs tested: tools/go-modules.py <changed path>... (- reads them from stdin, one a line; --all <why>
selects every module) prints each selected module's directory under go/ (".", "kit", ...) and, on stderr, one line per module
saying why it is tested or skipped.

A changed file under go/<module>/ selects that module and every module that requires it, read from the go.mod files. Another
file under go/ is the daemon's. go.work, a go.mod or go.sum, check.yml, and the scripts that pick or run the Go tests select all.
Outside go/, contract/ and tests/ select all, and what one module's tests read selects that module (READ_BY); anything else
selects none. A test that scans the source of every module belongs in the daemon's, which every change under go/ selects."""
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PREFIX = "github.com/handline/handline/go"
# What changes how every module is built or tested, or which of them are.
ALL_FILES = {
    "go/go.work", "go/go.work.sum", ".github/workflows/check.yml", "tools/check.sh", "tools/go-modules.py", "tools/go-shard.sh",
    "tools/go-affected.py", "tools/test-go.sh", "tools/go-env.sh", "tools/check-go-tidy.sh", "tools/check-go-dupl.sh",
    "tools/leakcheck.py", "tools/ci-tmux.sh", "tools/live_account.py",
}
ALL_DIRS = ("contract/", "tests/")
# Files outside go/ that one module's tests read, by path prefix: go/data/team's protocol test holds them to the protocol block.
READ_BY = {"AGENTS.md": "data", "skill/": "data"}


def modules(root: str = ROOT) -> dict[str, set[str]]:
    """Each module's directory under go/ ("." for the daemon's) and the modules of this repo its go.mod requires."""
    out = {}
    with open(os.path.join(root, "go", "go.work")) as f:
        work = re.sub(r"//.*", "", f.read())
    # use ./x, or use ( ... ) with one directory a line, quoted or not.
    uses = re.findall(r"^\s*use\s+(\S+)\s*$", work, re.M) + [
        ln for block in re.findall(r"^\s*use\s*\((.*?)\)", work, re.M | re.S) for ln in block.split()]
    dirs = [os.path.normpath(u.strip('"`')) for u in uses if u.strip('"`') not in ("(", ")")]
    for d in dirs:
        with open(os.path.join(root, "go", d, "go.mod")) as f:
            text = f.read()
        out[d] = {m for m in re.findall(rf"^\s*(?:require\s+)?{re.escape(PREFIX)}/(\S+)\s+v", text, re.M)}
    return out


def select(paths: list[str], graph: dict[str, set[str]]) -> dict[str, str]:
    """The selected modules, each with why."""
    why: dict[str, str] = {}
    nested = sorted((d for d in graph if d != "."), key=len, reverse=True)
    for p in paths:
        if p in ALL_FILES or p.startswith(ALL_DIRS) or (p.startswith("go/") and os.path.basename(p) in ("go.mod", "go.sum")):
            return {d: f"{p} changed, which every module is built or tested by" for d in graph}
        if not p.startswith("go/"):
            for prefix, d in READ_BY.items():
                if p.startswith(prefix) and d in graph:
                    why.setdefault(d, f"{p} changed, which go/{d}'s tests read")
            continue
        own = next((d for d in nested if p.startswith(f"go/{d}/")), ".")
        why.setdefault(own, f"{p} changed")
    todo = list(why)
    while todo:
        d = todo.pop()
        for user, needs in graph.items():
            if d in needs and user not in why:
                why[user] = f"it requires {d}"
                todo.append(user)
    return why


def name(d: str) -> str:
    return "daemon (go/)" if d == "." else f"go/{d}"


def main(args: list[str]) -> None:
    graph = modules()
    if args[:1] == ["--all"]:
        why = {d: " ".join(args[1:]) or "every module was asked for" for d in graph}
    else:
        why = select([ln for ln in sys.stdin.read().splitlines() if ln] if args == ["-"] else args, graph)
    for d in sorted(graph):
        print(f"go modules: {name(d)}: " + (f"tested: {why[d]}" if d in why else "skipped: no change reaches it"), file=sys.stderr)
    for d in sorted(why):
        print(d)


if __name__ == "__main__":
    main(sys.argv[1:])
