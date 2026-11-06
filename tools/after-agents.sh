#!/usr/bin/env bash
# The founder's steps once every agent is idle: close the sandbox gaps in the shared settings; running sessions then need hl close.
#   tools/after-agents.sh            show what would change and what is still open
#   tools/after-agents.sh --apply    back the settings up, then make the changes
# Run it from your own terminal, not an agent's: the settings file (~/.handline/shared/.claude/settings.json) is the founder's.
# It removes the Handline entries from sandbox.excludedCommands, and the main checkout and ~/.local/bin from
# permissions.additionalDirectories; the main checkout's .git stays writable, as agents commit through it.
set -euo pipefail

apply=0
[ "${1:-}" = "--apply" ] && apply=1
home="${HANDLINE_HOME:-$HOME/.handline}"
# The main checkout: the folder that holds the git directory this script's worktree shares.
main_checkout="$(dirname "$(git -C "$(dirname "$0")" rev-parse --path-format=absolute --git-common-dir)")"
settings="$home/shared/.claude/settings.json"
[ -f "$settings" ] || { echo "no settings file at $settings" >&2; exit 1; }

busy="$(hl ps | awk 'NR>3 && $1!="AGENT" && $3!="IDLE" && $3!="PENDING" && $3!="" && $1 ~ /^[A-Za-z0-9._-]+$/ && $3!="CLOSED" {print $1" "$3}' | head -20 || true)"
if [ -n "$busy" ]; then
    echo "These agents are not idle. Wait for them, or accept that their sessions keep the old settings:"
    echo "$busy" | sed 's/^/  /'
fi

python3 - "$settings" "$apply" "$main_checkout" <<'PY'
import json, shutil, sys
path, apply, main_checkout = sys.argv[1], sys.argv[2] == "1", sys.argv[3]
data = json.load(open(path))
sb = data.get("sandbox", {})
perm = data.get("permissions", {})
excluded = sb.get("excludedCommands", [])
dirs = perm.get("additionalDirectories", [])
drop_ex = [e for e in excluded if isinstance(e, str) and e.split(" ", 1)[0] in ("hl", "handline")]
home_bin = {d for d in dirs if d.endswith("/.local/bin") or d.rstrip("/") == main_checkout}
print("excludedCommands to remove:", drop_ex or "none")
print("additionalDirectories to remove:", sorted(home_bin) or "none")
if not (drop_ex or home_bin):
    print("settings already closed")
    sys.exit(0)
if not apply:
    print("(dry run: nothing changed; add --apply)")
    sys.exit(0)
shutil.copy2(path, path + ".bak")
sb["excludedCommands"] = [e for e in excluded if e not in drop_ex]
perm["additionalDirectories"] = [d for d in dirs if d not in home_bin]
json.dump(data, open(path, "w"), indent=2)
open(path, "a").write("\n")
print("done; backup at", path + ".bak")
PY

echo
echo "Still manual, in this order:"
echo "  1. Relaunch the team: for a in \$(hl ps | awk '\$3==\"IDLE\"{print \$1}'); do hl close \"\$a\"; done"
echo "  2. hl doctor            (expect: Handline runs inside the agents' sandbox)"
echo "  3. Ask one relaunched agent to run the probe from the rollout guide."
