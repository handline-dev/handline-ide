#!/usr/bin/env python3
"""Every `hl <verb> [sub]` that the live docs, AGENTS.md, skill/ and the agent prompts name exists in go/cmd/hl.

    python3 tools/check_hl_commands.py [FILE...]    the default files, or those named

Verbs come from the command table in go/cmd/hl/main.go, subcommands from the usage table in go/cmd/hl/usage.go. Only text in
backticks is read (and, in go/scheduler/lifecycle, string lines, not comments). History docs, which describe what was or
what is planned, are listed in HISTORY and skipped. Prints one line per problem and exits 1 when there is one."""
import glob
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HISTORY = {
    "agent-state-elsewhere", "contract-requests", "daemon-plan", "design-gaps", "direct-writers", "hook-probes", "improvement-round",
    "lint-and-file-size", "memory-design", "orca-cli-survey", "orca-feature-inventory", "pc-app-gaps", "phase0-findings",
    "python-daemon-removal", "screen-reading-inventory", "structured-switch", "terminal-pane", "turborepo-layout",
    "vendor-extension-decisions", "watch-vendors", "web-main-view", "web-ui-design",
}  # plus every story*: research and design written before the command existed
REF = re.compile(r"(?<![\w/.-])hl ([a-z][a-z-]*)(?: ([a-z][a-z-]*))?")


def read(path):
    with open(os.path.join(ROOT, path)) as f:
        return f.read()


def tables():
    """(verbs, subs): subs maps a verb to its subcommands where the usage table names them, as `<verb> <sub>` keys or as an
    alternation (`install|uninstall`) in its form; any other verb takes arguments, not subcommands."""
    verbs = set(re.findall(r'^\t"([a-z-]+)":\s+\w+,', read("go/cmd/hl/main.go"), re.M))
    subs = {}
    for key, form in re.findall(r'^\t"([a-z -]+)":\s*\{\s*`([^`]*)`', read("go/cmd/hl/usage.go"), re.M):
        verb, _, sub = key.partition(" ")
        if sub:
            subs.setdefault(verb, set()).add(sub)
        for alt in re.findall(r"[a-z-]+(?:\|[a-z-]+)+", form):
            subs.setdefault(verb, set()).update(alt.split("|"))
    return verbs, subs


# The prompts the daemon types into sessions: the commands they name are checked too.
LIFECYCLE = "go/scheduler/lifecycle"


def files():
    out = ["AGENTS.md"] + sorted(glob.glob(os.path.join(ROOT, "skill", "*")))
    for d in sorted(glob.glob(os.path.join(ROOT, "docs", "*.md"))):
        name = os.path.basename(d)[:-3]
        if name not in HISTORY and not name.startswith("story"):
            out.append(d)
    lifecycle = [g for g in sorted(glob.glob(os.path.join(ROOT, LIFECYCLE, "*.go"))) if not g.endswith("_test.go")]
    if not lifecycle:
        raise SystemExit(f"tools/check_hl_commands.py: no Go files in {LIFECYCLE}: it moved; point LIFECYCLE at it")
    out += lifecycle
    return [os.path.relpath(os.path.join(ROOT, f), ROOT) for f in out]


def problems(path, verbs, subs):
    out = []
    go = path.endswith(".go")
    for n, line in enumerate(read(path).splitlines(), 1):
        if go and line.lstrip().startswith("//"):
            continue
        for span in re.findall(r"`([^`]+)`", line):
            for m in REF.finditer(span):
                verb, sub = m.groups()
                if verb not in verbs:
                    out.append(f"{path}:{n}: `{m.group(0)}`: hl has no command {verb!r}")
                elif sub and verb in subs and sub not in subs[verb]:
                    out.append(f"{path}:{n}: `{m.group(0)}`: hl {verb} has no {sub!r}")
    return out


def main(argv):
    verbs, subs = tables()
    found = [p for f in (argv[1:] or files()) for p in problems(f, verbs, subs)]
    print("\n".join(found))
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
