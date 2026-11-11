#!/usr/bin/env python3
"""Generate contract/protocol.md, contract/protocol.ts, contract/gen/ and the Go daemon's method table
(go/protocol/rpc/methods_gen.go) from contract/spec/*.json.

The spec is one file per method group, so two changes to different groups touch different files: core.json holds the
x- sections and the errors, and each file holds its group's methods and the schemas only that group uses. load() joins
them; a name in two files is an error.

  python3 contract/generate.py            write the generated files
  python3 contract/generate.py --check    exit 1 if one is stale (tools/check.sh and CI run this)
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
SPECS = HERE / "spec"
GEN = HERE / "gen"
APP_GEN = HERE.parent / "packages" / "app-data" / "src" / "generated"
GO_TABLE = HERE.parent / "go" / "protocol" / "rpc" / "methods_gen.go"
HEADER = "Generated from contract/spec/ by contract/generate.py; do not edit."


def parts() -> dict[str, dict]:
    """Each spec file by its group, core first."""
    files = sorted(SPECS.glob("*.json"), key=lambda p: (p.stem != "core", p.stem))
    return {p.stem: json.loads(p.read_text()) for p in files}


def join(groups: dict[str, dict]) -> dict:
    """The whole spec, as one protocol.json would be."""
    spec: dict = {"methods": [], "components": {}}
    names: set[str] = set()
    for group, part in groups.items():
        for key, value in part.items():
            if key == "methods":
                for m in value:
                    if m["name"] in names:
                        raise SystemExit(f"spec/{group}.json: method {m['name']} is in another spec file too")
                    names.add(m["name"])
                spec["methods"] += value
            elif key == "components":
                for kind, items in value.items():
                    have = spec["components"].setdefault(kind, {})
                    if have.keys() & items.keys():
                        raise SystemExit(f"spec/{group}.json: {kind} {sorted(have.keys() & items.keys())} are in another spec file too")
                    have.update(items)
            elif key in spec:
                raise SystemExit(f"spec/{group}.json: {key} is in another spec file too")
            else:
                spec[key] = value
    return spec


def load() -> dict:
    return join(parts())


def refs(node) -> set[str]:
    """The schema names a part of the spec points at."""
    if isinstance(node, dict):
        out = {ref_name(node["$ref"])} if isinstance(node.get("$ref"), str) and node["$ref"].startswith("#/components/schemas/") else set()
        return out.union(*(refs(v) for v in node.values()))
    if isinstance(node, list):
        return set().union(*(refs(v) for v in node))
    return set()


def pascal(name: str) -> str:
    return "".join(p[:1].upper() + p[1:] for p in re.split(r"[._]", name))


def ref_name(ref: str) -> str:
    return ref.rsplit("/", 1)[1]


def ts(schema: dict, indent: int = 0) -> str:
    """A JSON Schema as a TypeScript type."""
    if "$ref" in schema:
        return ref_name(schema["$ref"])
    if "const" in schema:
        return json.dumps(schema["const"])
    if "enum" in schema:
        return " | ".join(json.dumps(v) for v in schema["enum"])
    for key, joiner in (("oneOf", " | "), ("anyOf", " | "), ("allOf", " & ")):
        if key in schema:
            return joiner.join(_wrap(ts(s, indent)) for s in schema[key])
    t = schema.get("type")
    if t == "string":
        return "string"
    if t in ("integer", "number"):
        return "number"
    if t == "boolean":
        return "boolean"
    if t == "null":
        return "null"
    if t == "array":
        items = ts(schema["items"], indent)
        # _wrap skips a type that starts with "{"; a composite one still needs parentheses.
        composite = any(k in schema["items"] for k in ("oneOf", "anyOf", "allOf"))
        return (f"({items})" if composite else _wrap(items)) + "[]"
    if t == "object":
        props = schema.get("properties")
        extra = schema.get("additionalProperties")
        if not props:
            return "{ [key: string]: %s }" % (ts(extra, indent) if isinstance(extra, dict) else "unknown") if extra is not False else "{}"
        req = set(schema.get("required", []))
        pad = "  " * (indent + 1)
        lines = []
        for k, v in props.items():
            if v.get("description"):
                lines.append(f"{pad}/** {v['description']} */")
            lines.append(f"{pad}{json.dumps(k) if not re.fullmatch(r'[A-Za-z_]\w*', k) else k}{'' if k in req else '?'}: {ts(v, indent + 1)};")
        if extra:
            lines.append(f"{pad}[key: string]: unknown;")
        return "{\n" + "\n".join(lines) + "\n" + "  " * indent + "}"
    return "unknown"


def _wrap(t: str) -> str:
    return f"({t})" if (" | " in t or " & " in t) and not t.startswith("{") else t


def params_type(m: dict) -> str:
    return ts({"type": "object", "properties": {p["name"]: p["schema"] for p in m["params"]},
               "required": [p["name"] for p in m["params"] if p["required"]]})


METHOD_INFO = "{ role: Role; callers: Role[] | null; write: boolean; since: number }"
CACHE_INFO = "{ resource: string; id?: string; feed: Topic | null } | false"
WRITE_INFO = "{ kind: WriteKind; label: string } | false"


def check_app(spec: dict) -> None:
    """Each read has x-app-cache and each write x-app-write (rules in core.json x-app)."""
    topics = set(spec["components"]["schemas"]["Topic"]["enum"])
    kinds = spec["x-app"]["kinds"]
    text = lambda v: isinstance(v, str) and v != ""
    bad = []
    for m in spec["methods"]:
        key, other = ("x-app-write", "x-app-cache") if m["x-write"] else ("x-app-cache", "x-app-write")
        v = m.get(key)
        if other in m:
            bad.append(f"{m['name']}: {other} on a {'write' if m['x-write'] else 'read'}")
        elif v is False:
            continue
        elif not isinstance(v, dict):
            bad.append(f"{m['name']}: no {key} (false when the app does not {'write' if m['x-write'] else 'cache'} it)")
        elif m["x-write"]:
            if v.keys() != {"kind", "label"} or v["kind"] not in kinds or not text(v["label"]):
                bad.append(f"{m['name']}: x-app-write is {{kind: {'|'.join(kinds)}, label}}")
        elif not ({"resource", "feed"} <= v.keys() <= {"resource", "id", "feed"}) or not text(v["resource"]):
            bad.append(f"{m['name']}: x-app-cache is {{resource, id?, feed}}, feed a topic or null")
        elif v["feed"] is not None and not (isinstance(v["feed"], str) and v["feed"] in topics):
            bad.append(f"{m['name']}: x-app-cache feed {v['feed']!r} is no topic")
        elif "id" in v and v["id"] not in {p["name"] for p in m["params"] if p["required"]}:
            bad.append(f"{m['name']}: x-app-cache id {v['id']!r} is no required param of it")
    if bad:
        raise SystemExit("contract/spec: " + "; ".join(bad))


def gen_group_ts(group: str, part: dict, owner: dict[str, str], spec: dict) -> str:
    """One group's types, its Methods interface and its method table; core also carries the protocol-wide ones."""
    out = []
    for name, s in part.get("components", {}).get("schemas", {}).items():
        if s.get("description"):
            out.append(f"/** {s['description']} */")
        out.append(f"export type {name} = {ts(s)};\n")
    P, U = pascal(group), group.upper()
    out.append(f"export interface {P}Methods {{")
    for m in part.get("methods", []):
        out.append(f"  {json.dumps(m['name'])}: {{ params: {params_type(m)}; result: {ts(m['result']['schema'], 1)} }};")
    out += ["}", ""]
    out.append(f"export const {U}_METHODS: {{ [K in keyof {P}Methods]: {METHOD_INFO} }} = {{")
    for m in part.get("methods", []):
        out.append(f"  {json.dumps(m['name'])}: {{ role: {json.dumps(m['x-role'])}, callers: {json.dumps(m.get('x-callers'))}, write: {json.dumps(m['x-write'])}, since: {m['x-since']} }},")
    out += ["};", ""]
    for key, U2, info, write in (("x-app-cache", "APP_CACHE", CACHE_INFO, False), ("x-app-write", "APP_WRITES", WRITE_INFO, True)):
        ms = [m for m in part.get("methods", []) if m["x-write"] == write]
        names = " | ".join(json.dumps(m["name"]) for m in ms) or "never"
        out.append(f"/** {key} of each {'write' if write else 'read'} (x-app in core.json). */")
        out.append(f"export const {U}_{U2}: {{ [K in {names}]: {info} }} = {{")
        out += [f"  {json.dumps(m['name'])}: {json.dumps(m[key])}," for m in ms]
        out += ["};", ""]
    if group == "core":
        out += [f"export const PROTOCOL = {spec['x-protocol']};", ""]
        codes = spec["components"]["errors"]
        out.append("export type ErrorCode = " + " | ".join(json.dumps(c) for c in codes) + ";")
        out.append("export const RPC_CODES: { [K in ErrorCode]: number | null } = {")
        out += [f"  {c}: {json.dumps(e['code'])}," for c, e in codes.items()]
        out += ["};", "", "export interface Notifications {"]
        for n, d in spec["x-notifications"].items():
            out.append(f"  {n}: {ts(d['params']) if isinstance(d['params'], dict) else d['params']};")
        out += ["}", "", "/** The state words the daemon sends; an open set, so a client keeps a fallback. */",
                "export const STATE_WORDS = " + json.dumps(list(spec["x-state-words"]["words"])) + " as const;", "",
                "/** x-status: the state and reason every agent and task carries; open sets for a client, which shows `text` for a word it does not know. */",
                "export const STATUS_STATES = " + json.dumps(list(spec["x-status"]["states"])) + " as const;",
                "export const STATUS_REASONS = " + json.dumps(list(spec["x-status"]["reasons"])) + " as const;", "",
                "export const ROLE_ORDER: Role[] = " + json.dumps(spec["x-roles"]["order"]) + ";", "",
                "export type WriteKind = " + " | ".join(json.dumps(k) for k in spec["x-app"]["kinds"]) + ";", ""]
    imports: dict[str, set[str]] = {}
    for name in refs(part) | {"Role", "Topic"}:
        if owner[name] != group:
            imports.setdefault(owner[name], set()).add(name)
    if group != "core":
        imports.setdefault("core", set()).add("WriteKind")
    head = [f"// {HEADER}", "/* eslint-disable */"]
    head += [f"import type {{ {', '.join(sorted(names))} }} from './{g}.ts';" for g, names in sorted(imports.items())]
    return "\n".join(head + [""] + out)


def gen_index_ts(groups: dict[str, dict]) -> str:
    """protocol.ts: every group's exports, and the Methods interface and method table of them all."""
    out = [f"// {HEADER}", "/* eslint-disable */", "import type { Role } from './gen/core.ts';"]
    out += [f"import {{ {g.upper()}_METHODS, {g.upper()}_APP_CACHE, {g.upper()}_APP_WRITES, type {pascal(g)}Methods }} from './gen/{g}.ts';" for g in groups]
    out += [""] + [f"export * from './gen/{g}.ts';" for g in groups] + [""]
    out += [f"export interface Methods extends {', '.join(pascal(g) + 'Methods' for g in groups)} {{}}", "",
            "export type MethodName = keyof Methods;", "",
            f"export const METHODS: {{ [K in MethodName]: {METHOD_INFO} }} = {{",
            *[f"  ...{g.upper()}_METHODS," for g in groups], "};", "",
            "/** How the app caches each read and writes each write (x-app in core.json). */",
            "export const APP_CACHE = {", *[f"  ...{g.upper()}_APP_CACHE," for g in groups], "};", "",
            "export const APP_WRITES = {", *[f"  ...{g.upper()}_APP_WRITES," for g in groups], "};", ""]
    return "\n".join(out)


def short(schema: dict) -> str:
    """A result type for a table cell: named types by name, inline objects as {...}."""
    if "$ref" in schema:
        return ref_name(schema["$ref"])
    for key, joiner in (("allOf", " & "), ("oneOf", " | "), ("anyOf", " | ")):
        if key in schema:
            return joiner.join(short(s) for s in schema[key])
    if schema.get("type") == "object":
        return "{" + ", ".join(schema.get("properties", {})) + "}" if schema.get("properties") else "object"
    return ts(schema).split("\n")[0]


def cell(t: str) -> str:
    return t.replace("|", "\\|").replace("\n", " ")


def gen_md(spec: dict, groups: dict[str, dict]) -> str:
    """protocol.md: everything but the methods, which each group's file lists."""
    fr = spec["x-framing"]
    o = [f"<!-- {HEADER} -->", f"# {spec['info']['title']} (protocol {spec['x-protocol']})", "", spec["info"]["description"], "",
         "## Framing", ""]
    o += [f"- **Transport:** {'; '.join(fr['transport'])}."] + [f"- **{k.replace('_', ' ').capitalize()}:** {fr[k]}" for k in ("reach", "lines", "ids", "concurrency", "network", "params", "request_id")]
    o += ["", "## Handshake and versions", "", spec["x-handshake"], "", "## Roles", "",
          f"Order: {' < '.join(spec['x-roles']['order'])}. {spec['x-roles']['rule']}", "", "## Errors", "",
          "Every error is a JSON-RPC error whose `data.code` is the stable string. A refusal carries the reason; fail-closed is `refused`, never a silent default.", "",
          "| Code | RPC code | data |", "|---|---|---|"]
    o += [f"| `{c}` | {e['code'] if e['code'] is not None else 'none (client side)'} | {cell(e['data'])} |" for c, e in spec["components"]["errors"].items()]
    o += ["", "## Gates", "", "| Gate | Refuses when |", "|---|---|"] + [f"| `{g}` | {cell(t)} |" for g, t in spec["x-gates"].items()]
    app = spec["x-app"]
    o += ["", "## App cache and writes", "", app["rule"], "", "- **x-app-cache:** " + app["cache"], "- **x-app-write:** " + app["write"]]
    o += [f"- **{k}:** {t}" for k, t in app["kinds"].items()]
    o += ["", "## Methods", "", "One file per group, generated from `spec/<group>.json`; field types are in `protocol.ts`.", ""]
    o += [f"- [{g}](gen/{g}.md): {len(p.get('methods', []))} methods" for g, p in groups.items()] + [""]
    ident = spec["x-identity"]
    o += ["## Who the caller is", "", ident["rule"], "", "- **Sender:** " + ident["sender"], "- **Ownership:** " + ident["ownership"], "",
          "## Agent commands", "", "Every `hl` command an agent runs today and the method that serves it.", "", "| Command | Method |", "|---|---|"]
    o += [f"| `{c}` | `{m}` |" for c, m in spec["x-agent-commands"].items()]
    d = spec["x-dropped-from-agents"]
    o += ["", "## Dropped from agents on purpose", "", d["note"], ""]
    o += [f"- **{k.replace('_', ' ').capitalize()}:** " + "; ".join(f"`{x}`" for x in v) for k, v in d.items() if k != "note"]
    o += ["", spec["x-note-topics"], ""]
    o += ["## Topics", "", "| Topic | Event types (data) | Snapshot method |", "|---|---|---|"]
    for t, d in spec["x-topics"].items():
        o.append(f"| `{t}` | {', '.join(f'`{k}` ({v})' for k, v in d['data'].items())} | `{d['snapshot']}` |")
    sw = spec["x-state-words"]
    o += ["", "## State words", "", sw["rule"], "", "| Word | Meaning |", "|---|---|"] + [f"| `{w}` | {cell(m)} |" for w, m in sw["words"].items()]
    o += ["", "What the daemon sends for each of its own states:", "", "| Daemon state | Word |", "|---|---|"] + [f"| {cell(k)} | `{v}` |" for k, v in sw["from_daemon"].items()]
    o += ["", sw["pending"]]
    st = spec["x-status"]
    o += ["", "## Status", "", st["rule"], "", "| State | Meaning |", "|---|---|"] + [f"| `{w}` | {cell(m)} |" for w, m in st["states"].items()]
    o += ["", "| Reason | Meaning |", "|---|---|"] + [f"| `{w}` | {cell(m)} |" for w, m in st["reasons"].items()]
    o += ["", "## Notifications", ""]
    for n, d in spec["x-notifications"].items():
        o.append(f"- `{n}`: {d.get('note', '')} Params: `{ts(d['params']).replace(chr(10), ' ') if isinstance(d['params'], dict) else d['params']}`")
    o += ["", "## Terminal channel", "", spec["x-terminal"], "", "### Format bytes", ""] + [f"- **{k.replace('_', ' ').capitalize()}.** {v}" for k, v in spec["x-terminal-bytes"].items()]
    o += ["", "### Format styled", ""] + [f"- **{k.replace('_', ' ').capitalize()}.** {v}" for k, v in spec["x-terminal-styled"].items()]
    o += ["", "## Delivery to a structured agent", ""] + [f"- **{k.replace('_', ' ').capitalize()}.** {v}" for k, v in spec["x-delivery"].items()]
    o += ["", "## Driving a structured agent", ""] + [f"- **{k.replace('_', ' ').capitalize()}.** {v}" for k, v in spec["x-structured-controls"].items()]
    return "\n".join(o) + "\n"


def app_cell(m: dict) -> str:
    """The x-app-cache or x-app-write of a method, short: `agent` by agent, fed by agents; authored: send."""
    v = m["x-app-write"] if m["x-write"] else m["x-app-cache"]
    if v is False:
        return "not cached" if not m["x-write"] else "not from the app"
    if m["x-write"]:
        return f"{v['kind']}: {v['label']}"
    return f"`{v['resource']}`" + (f" by {v['id']}" if "id" in v else "") + (f", fed by {v['feed']}" if v["feed"] else ", snapshot")


def gen_group_md(group: str, part: dict, spec: dict) -> str:
    """One group's methods: the table, then each method's summary."""
    o = [f"<!-- {HEADER} -->", f"# {group} methods (protocol {spec['x-protocol']})", "",
         "A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.", "",
         "| Method | Role | Write | Params | Result | Errors | Gates | App |", "|---|---|---|---|---|---|---|---|"]
    for m in part.get("methods", []):
        ps = ", ".join(f"`{p['name']}{'' if p['required'] else '?'}`" for p in m["params"]) or "none"
        errs = ", ".join(f"`{ref_name(e['$ref'])}`" for e in m.get("errors", [])) or ""
        o.append(f"| `{m['name']}` | {m['x-role']}{' (only ' + ', '.join(m['x-callers']) + ')' if m.get('x-callers') else ''} | {'yes' if m['x-write'] else ''} | {ps} | `{cell(short(m['result']['schema']))}` | {errs} | {', '.join(m.get('x-gates', []))} | {app_cell(m)} |")
    o.append("")
    for m in part.get("methods", []):
        o += [f"### `{m['name']}`", "", m["summary"] + (" " + m["description"] if m.get("description") else ""), ""]
    return "\n".join(o) + "\n"


def hook_name(method: str) -> str:
    return "use" + "".join(w[:1].upper() + w[1:] for w in re.split(r"[._]", method))


def event_fields(spec: dict) -> dict[str, list[str]]:
    """The fields each pushed event's data must carry: a schema's required list, or the non-optional names of an inline {a, b?}."""
    schemas = spec["components"]["schemas"]
    out: dict[str, list[str]] = {}
    for topic in spec["x-topics"].values():
        for event, shape in topic["data"].items():
            if shape in schemas:
                req = schemas[shape].get("required")
            elif shape.startswith("{"):
                req = [n.strip() for n in shape.strip("{}").split(",") if n.strip() and not n.strip().endswith("?")]
            else:
                continue
            if req:
                out[event] = list(req)
    return out


def gen_app_data(spec: dict) -> dict[Path, str]:
    """packages/app-data/src/generated/: the reads table, one hook per read and per cached write, the enum unions and the event guards."""
    head = f"// {HEADER}\n"
    reads = [m for m in spec["methods"] if not m["x-write"] and m["x-app-cache"] is not False]
    writes = [m for m in spec["methods"] if m["x-write"] and m["x-app-write"] is not False and m["x-app-write"]["kind"] != "uncached"]
    table = []
    for m in reads:
        c = m["x-app-cache"]
        parts = [f"resource: {json.dumps(c['resource'])}"] + ([f"id: {json.dumps(c['id'])}"] if "id" in c else []) + ([f"feed: {json.dumps(c['feed'])}"] if c["feed"] else [])
        table.append(f"  {json.dumps(m['name'])}: {{ {', '.join(parts)} }},")
    r = head + "import type { Topic } from '@handline/client';\n\n"
    r += "/** The cache entry of each read (x-app-cache): its name in the key, the param that names the entry, the topic that keeps it fresh. */\n"
    r += "export const reads = {\n" + "\n".join(table) + "\n} as const satisfies Record<string, { resource: string; id?: string; feed?: Topic }>;\n\n"
    r += "/** The reads the app does not cache (x-app-cache false). */\nexport const NOT_CACHED = " + json.dumps([m["name"] for m in spec["methods"] if not m["x-write"] and m["x-app-cache"] is False]) + " as const;\n"
    r += "\n/** The writes the app does not make from a screen (x-app-write false). */\nexport const NOT_WRITTEN = " + json.dumps([m["name"] for m in spec["methods"] if m["x-write"] and m["x-app-write"] is False]) + " as const;\n"
    r += "\n/** What each event's data must carry (the schema's required fields); the bridge reads the topic again for one that does not. */\nexport const EVENT_REQUIRED: Record<string, readonly string[]> = " + json.dumps(event_fields(spec), indent=2) + ";\n"
    h = head + "// One hook per daemon read and per write the app caches; useRead and useWrite are the layer under them.\n"
    h += "import type { CallParams } from '@handline/client';\nimport { useRead } from '#src/read.ts';\nimport { useWrite } from '#src/write.ts';\n\n"
    h += "type ReadOpts = NonNullable<Parameters<typeof useRead>[2]>;\n\n"
    for m in reads:
        n = json.dumps(m["name"])
        if any(p["required"] for p in m["params"]):
            h += f"export const {hook_name(m['name'])} = (params: CallParams<{n}>, opts?: ReadOpts) => useRead({n}, params, opts);\n"
        else:
            h += f"export const {hook_name(m['name'])} = (params: CallParams<{n}> = {{}}, opts?: ReadOpts) => useRead({n}, params, opts);\n"
    h += "\n"
    for m in writes:
        h += f"export const {hook_name(m['name'])} = (target?: string) => useWrite({json.dumps(m['name'])}, target);\n"
    e = head
    for name, words in (("StateWord", spec["x-state-words"]["words"]), ("StatusState", spec["x-status"]["states"]), ("StatusReason", spec["x-status"]["reasons"])):
        e += f"export type {name} = {' | '.join(json.dumps(w) for w in words)};\n"
    return {APP_GEN / "reads.ts": r, APP_GEN / "hooks.ts": h, APP_GEN / "enums.ts": e}


GO_TYPES = {"string": "string", "integer": "json.Number", "number": "json.Number", "boolean": "bool", "array": "[]any", "object": "map[string]any"}


def go_type(schema: dict, schemas: dict) -> str:
    """The Go value a handler gets for a param, as the connection decodes JSON (numbers as json.Number); any for a union."""
    while "$ref" in schema:
        schema = schemas[ref_name(schema["$ref"])]
    if "enum" in schema or "const" in schema:
        values = schema.get("enum", [schema.get("const")])
        kinds = {type(v).__name__ for v in values}
        return {"str": "string", "int": "json.Number", "bool": "bool"}.get(kinds.pop(), "any") if len(kinds) == 1 else "any"
    return GO_TYPES.get(schema.get("type"), "any")


def who_props(schema: dict, schemas: dict) -> list:
    """The properties of an object param that name a who, resolved like a who param (one level down: a filter's owner)."""
    while "$ref" in schema and not schema["$ref"].endswith("/AgentName"):
        schema = schemas[ref_name(schema["$ref"])]
    return sorted(k for k, v in schema.get("properties", {}).items() if is_who(v))


def is_who(schema: dict) -> bool:
    """A param that names a who (AgentName, or a list of them): the daemon resolves its words to keys before the handler."""
    return schema.get("$ref", schema.get("items", {}).get("$ref", "")).endswith("/AgentName")


def gen_go(spec: dict) -> str:
    """go/protocol/rpc/methods_gen.go: the method table Register takes each method's role and write flag from."""
    schemas = spec["components"]["schemas"]
    o = ["// Code generated by contract/generate.py from contract/spec/; DO NOT EDIT.", "", "package rpc", "",
         "// specMethods is every method of the contract, in the spec's order.", "var specMethods = []SpecMethod{"]
    q = json.dumps  # a Go string, bool or number literal is its JSON
    for m in spec["methods"]:
        ps = ", ".join(f"{{Name: {q(p['name'])}, Type: {q(go_type(p['schema'], schemas))}, Required: {q(p['required'])}"
                       + (", Who: true" if is_who(p["schema"]) else "")
                       + (", WhoIn: []string{" + ", ".join(map(q, who_props(p["schema"], schemas))) + "}" if who_props(p["schema"], schemas) else "")
                       + "}" for p in m["params"])
        callers = "[]string{" + ", ".join(map(q, m["x-callers"])) + "}" if m.get("x-callers") else "nil"
        res = m["result"]["schema"]
        result = ref_name(res["$ref"]) if "$ref" in res else ""
        o.append(f"\t{{Name: {q(m['name'])}, Role: {q(m['x-role'])}, Write: {q(m['x-write'])}, Since: {m['x-since']}, Callers: {callers}, "
                 f"Params: []SpecParam{{{ps}}}, Result: {q(result)}}},")
    o += ["}", ""]
    return "\n".join(o)


def outputs() -> dict[Path, str]:
    groups = parts()
    spec = join(groups)
    check_app(spec)
    owner = {name: g for g, p in groups.items() for name in p.get("components", {}).get("schemas", {})}
    out = {HERE / "protocol.ts": gen_index_ts(groups), HERE / "protocol.md": gen_md(spec, groups), GO_TABLE: gen_go(spec)}
    for g, p in groups.items():
        out[GEN / f"{g}.ts"] = gen_group_ts(g, p, owner, spec)
        out[GEN / f"{g}.md"] = gen_group_md(g, p, spec)
    out.update(gen_app_data(spec))
    return out


def main(argv: list[str]) -> int:
    want = outputs()
    stale = [p for p, text in want.items() if not p.exists() or p.read_text() != text]
    APP_GEN.mkdir(parents=True, exist_ok=True)
    gone = [p for p in GEN.glob("*") if p not in want]
    if "--check" in argv:
        for p in stale + gone:
            print(f"stale: {p.relative_to(HERE.parent)}; run python3 contract/generate.py", file=sys.stderr)
        return 1 if stale or gone else 0
    GEN.mkdir(exist_ok=True)
    for p in gone:
        p.unlink()
    for p, text in want.items():
        p.write_text(text)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
