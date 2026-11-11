#!/usr/bin/env python3
"""The account a live run spends, named by HANDLINE_LIVE_ACCOUNT.

  HANDLINE_LIVE_ACCOUNT=name python3 tools/live_account.py      prints {"account", "home", "config_dir", "why"} as JSON

tools/live_e2e.py and the Go live tests (go/accounts/livetest) both pick through it.
"""
from __future__ import annotations

import json
import os
import sqlite3
import sys
from pathlib import Path


def account_key(home: str, name: str) -> str:
    """The account's key (c<id>), which names its folder under accounts/; SystemExit when the home has no such account."""
    db = Path(home) / "db.sqlite"
    try:
        with sqlite3.connect(f"file:{db}?mode=ro", uri=True) as c:
            row = c.execute("SELECT id FROM accounts WHERE name = ?", (name,)).fetchone()
    except sqlite3.Error as e:
        raise SystemExit(f"live-account: cannot read {db}: {e}")
    if not row:
        raise SystemExit(f"live-account: no account {name!r} in {home}")
    return f"c{row[0]}"


def pick_live() -> dict:
    """{account, home, config_dir, why} for the account to spend, named by HANDLINE_LIVE_ACCOUNT; SystemExit without one."""
    name = os.environ.get("HANDLINE_LIVE_ACCOUNT", "")
    if not name:
        raise SystemExit("live-account: name the account to spend in HANDLINE_LIVE_ACCOUNT")
    home = os.environ.get("HANDLINE_HOME") or str(Path.home() / ".handline")
    return {"account": name, "home": home, "config_dir": str(Path(home) / "accounts" / account_key(home, name) / ".claude"), "why": "named by HANDLINE_LIVE_ACCOUNT"}


def main() -> int:
    print(json.dumps(pick_live()))
    return 0


if __name__ == "__main__":
    sys.exit(main())
