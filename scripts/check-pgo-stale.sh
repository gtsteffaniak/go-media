#!/usr/bin/env bash
# Warn when hot-path sources changed since default.pgo was generated (profile may be stale).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -s default.pgo ]]; then
	echo "::warning::default.pgo is missing or empty; run make pgo" >&2
	exit 0
fi

PROV="$ROOT/default.pgo.provenance.json"
if [[ ! -f "$PROV" ]]; then
	echo "::warning::default.pgo.provenance.json missing; regenerate with make pgo" >&2
	exit 0
fi

HEAD=$(python3 -c "import json; print(json.load(open('$PROV'))['gitHead'])" 2>/dev/null || echo "")
if [[ -z "$HEAD" || "$HEAD" == "unknown" ]]; then
	echo "::warning::PGO provenance has no gitHead; cannot detect staleness" >&2
	exit 0
fi

if ! git cat-file -e "$HEAD^{commit}" 2>/dev/null; then
	echo "::warning::PGO provenance gitHead $HEAD not in this checkout" >&2
	exit 0
fi

DIRS=$(python3 -c "
import json
for g in json.load(open('$PROV')).get('hotPathGlobs',[]):
    d=g.split('/**')[0].rstrip('/')
    if d: print(d)
")
CHANGED=0
while IFS= read -r dir; do
	[[ -z "$dir" ]] && continue
	while IFS= read -r f; do
		echo "PGO stale candidate: $f (changed since profile commit ${HEAD:0:7})" >&2
		CHANGED=1
	done < <(git diff --name-only "$HEAD" HEAD -- "$dir" 2>/dev/null || true)
done <<<"$DIRS"

if [[ "$CHANGED" -eq 1 ]]; then
	echo "::warning::Hot-path code changed since default.pgo was generated; run make pgo and commit default.pgo + default.pgo.provenance.json" >&2
else
	echo "PGO profile freshness: OK (no hot-path changes since ${HEAD:0:7})"
fi

exit 0
