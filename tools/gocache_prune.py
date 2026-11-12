#!/usr/bin/env python3
"""Keeps the shared Go build cache under a size cap: tools/gocache_prune.py [--dir DIR] [--cap-gb N] [--min-age-minutes M]
Over the cap, deletes the files nobody used longest (go marks an entry used by touching its mtime) until it is under again, never one
used within the last M minutes (a build in another worktree may be reading it). One run at a time; tools/go-env.sh starts it."""
from __future__ import annotations

import argparse
import fcntl
import os
import subprocess
import sys
import time
from pathlib import Path


def prune(cache: Path, cap_bytes: int, min_age_s: float, now: float | None = None) -> int:
    """Bytes deleted. Top-level files (README, trim.txt) and our own marker are not cache entries."""
    now = time.time() if now is None else now
    files, total = [], 0
    for sub in cache.iterdir():
        if not sub.is_dir():
            continue
        for p in sub.iterdir():
            try:
                st = p.stat()
            except OSError:
                continue
            total += st.st_size
            files.append((st.st_mtime, st.st_size, p))
    freed = 0
    for mtime, size, p in sorted(files):
        if total - freed <= cap_bytes or now - mtime < min_age_s:
            break
        try:
            p.unlink()
            freed += size
        except OSError:
            pass
    return freed


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir")
    ap.add_argument("--cap-gb", type=float, default=float(os.environ.get("HANDLINE_GOCACHE_MAX_GB", "20")))
    ap.add_argument("--min-age-minutes", type=float, default=60)
    a = ap.parse_args()
    cache = Path(a.dir or subprocess.run(["go", "env", "GOCACHE"], capture_output=True, text=True).stdout.strip())
    if not cache.is_dir():
        return 0
    with open(cache / "handline-prune.lock", "w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            return 0
        freed = prune(cache, int(a.cap_gb * 2**30), a.min_age_minutes * 60)
    print(f"gocache_prune: freed {freed / 2**30:.1f} GB from {cache}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
