#!/usr/bin/env bash
# Fails when the apps and packages hold more copied code than today: the share of duplicated lines may only go down.
# Lower MAX when a slice of the list lands (jscpd: 3 clones, 0.06% of lines, 2026-11-06). apps/web/legacy is the frozen old web,
# not linted either: the new web moves code out of it, so its copies are not counted.
cd "$(dirname "$0")/.." || exit 1
MAX=0.1
exec npx --yes jscpd@4.0.5 apps packages --min-lines 8 --min-tokens 60 --threshold "$MAX" --silent \
    --ignore "**/node_modules/**,**/dist/**,**/test/**,**/*.json,**/fixtures.ts,apps/web/legacy/**" --format typescript,tsx
