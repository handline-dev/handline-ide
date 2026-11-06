#!/usr/bin/env python3
"""The Go packages a change touches: tools/go-affected.py <changed path>... prints the reverse-dependency closure, one
`go test` pattern per line (./internal/x), or `ALL` when the change needs the full run: go.mod, go.sum, a migration, a
path outside go/ that is neither a doc nor a JS app file, or a changed directory that is no longer a package."""
import json
import subprocess
import sys

JS_ONLY = ("apps/", "web/", "packages/", "docs/", "design/")


def full(path):
    if path.startswith("go/"):
        rest = path[3:]
        return rest in ("go.mod", "go.sum") or "migrations/" in rest
    if path.startswith(JS_ONLY) or path.endswith(".md"):
        return False
    return True


def main(paths):
    if any(full(p) for p in paths):
        return ["ALL"]
    out = subprocess.run(["go", "list", "-json", "./..."], cwd="go", capture_output=True, text=True, check=True).stdout
    dec, pkgs, i = json.JSONDecoder(), {}, 0
    while i < len(out):
        obj, j = dec.raw_decode(out, i)
        pkgs[obj["Dir"]] = obj
        i = j
        while i < len(out) and out[i].isspace():
            i += 1
    root = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True).stdout.strip()
    prefix = next(iter(pkgs.values()))["Module"]["Path"]
    importers = {}
    for p in pkgs.values():
        for imp in set(p.get("Imports", []) + p.get("TestImports", []) + p.get("XTestImports", [])):
            importers.setdefault(imp, set()).add(p["ImportPath"])
    by_path = {p["ImportPath"]: p for p in pkgs.values()}
    todo = []
    for path in paths:
        if not path.startswith("go/"):
            continue
        d = root + "/" + path.rsplit("/", 1)[0] if "/" in path else root
        # testdata and other files under a package belong to the nearest package above them
        while d.startswith(root + "/go") and d not in pkgs:
            d = d.rsplit("/", 1)[0]
        if d not in pkgs:
            return ["ALL"]
        todo.append(pkgs[d]["ImportPath"])
    seen = set()
    while todo:
        ip = todo.pop()
        if ip in seen:
            continue
        seen.add(ip)
        todo.extend(importers.get(ip, ()))
    return sorted("." + ip[len(prefix):] for ip in seen if ip in by_path)


if __name__ == "__main__":
    print("\n".join(main(sys.argv[1:])))
