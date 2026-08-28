#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
	printf 'usage: %s [OUTPUT-DIR]\n' "$0" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$#" -eq 1 ]; then
	output_dir=$1
else
	output_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-pico-filters.XXXXXX")
fi
mkdir -p "$output_dir"

case_passes() {
	result=$1/result.json
	[ -s "$result" ] && python3 -c \
		'import json,sys; d=json.load(open(sys.argv[1])); raise SystemExit(d["summary"]["failures"] != 0)' \
		"$result"
}

run_case() {
	name=$1
	shift
	case_dir=$output_dir/$name
	if case_passes "$case_dir"; then
		printf '%s: existing passing result\n' "$name"
		return
	fi
	# The single-capture runner deliberately refuses to overwrite reference
	# audio. Preserve a failed/partial attempt for diagnosis, then give the
	# canonical case path a clean retry. The exhaustive isolated NOIZ runner
	# stores resumable references below references/, so leave those in place
	# until it has produced a result.
	if [ -s "$case_dir/result.json" ] || [ -s "$case_dir/pico-notes.wav" ]; then
		attempt=1
		archive=$case_dir.failed-$attempt
		while [ -e "$archive" ]; do
			attempt=$((attempt + 1))
			archive=$case_dir.failed-$attempt
		done
		mv "$case_dir" "$archive"
		printf '%s: archived incomplete/failed attempt as %s\n' "$name" "$archive"
	fi
	"$@" "$case_dir"
}

# NOIZ only changes PICO's noise instrument. Its colored-noise comparison
# needs long observations and fresh PICO processes so stochastic phase from a
# previous pitch cannot contaminate the next case.
run_case noiz-noise-all env PICO_SFX_NOIZ=1 PICO_SFX_SPEED=240 \
	"$script_dir/run_pico_sfx_isolated.sh" noise

# BUZZ is waveform-specific, so cover every PICO instrument at concert C.
# A two-second note also gives the stochastic noise branch stable resolution.
for waveform in triangle tilted-saw saw square pulse organ noise phaser; do
	run_case "buzz-$waveform-p36" env PICO_SFX_BUZZ=1 PICO_SFX_SPEED=240 \
		PICO_SFX_PITCH=36 "$script_dir/run_pico_sfx_notes.sh" "$waveform"
done

# DETUNE has instrument-specific branches (especially triangle, organ, noise,
# and phaser). Exercise both encoded levels for every instrument with a two-
# second note so close beating frequencies are observable.
for level in 1 2; do
	for waveform in triangle tilted-saw saw square pulse organ noise phaser; do
		run_case "detune$level-$waveform-p36" env PICO_SFX_DETUNE=$level \
			PICO_SFX_SPEED=240 PICO_SFX_PITCH=36 \
			"$script_dir/run_pico_sfx_notes.sh" "$waveform"
	done
done

# Reverb and dampen are channel filters after synthesis, so one harmonic-rich
# source is sufficient for both levels. Reverb uses a short impulse-like note
# to expose its complete tail; dampen uses a long saw to expose its pole.
for level in 1 2; do
	run_case "reverb$level-triangle-p36" env PICO_SFX_REVERB=$level \
		PICO_SFX_SPEED=16 PICO_SFX_PITCH=36 \
		"$script_dir/run_pico_sfx_notes.sh" triangle
	run_case "dampen$level-saw-p36" env PICO_SFX_DAMPEN=$level \
		PICO_SFX_SPEED=120 PICO_SFX_PITCH=36 \
		"$script_dir/run_pico_sfx_notes.sh" saw
done

python3 - "$output_dir" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
rows = []
for result_path in sorted(root.glob("*/result.json")):
    if ".failed-" in result_path.parent.name:
        continue
    result = json.loads(result_path.read_text(encoding="utf-8"))
    rows.append(
        {
            "case": result_path.parent.name,
            "cases": int(result["summary"]["cases"]),
            "failures": int(result["summary"]["failures"]),
            "metrics": {
                key: value
                for key, value in result["summary"].items()
                if key not in {"cases", "failures"}
            },
        }
    )
expected_groups = 29
summary = {
    "version": 1,
    "groups": len(rows),
    "cases": sum(row["cases"] for row in rows),
    "failures": sum(row["failures"] for row in rows),
    "results": rows,
}
(root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
if len(rows) != expected_groups or summary["failures"]:
    raise SystemExit(
        f"filter conformance incomplete: {len(rows)}/{expected_groups} groups, "
        f"{summary['failures']} failures"
    )
print(
    f"filter conformance: {summary['cases']}/{summary['cases']} cases pass "
    f"across {summary['groups']} groups"
)
PY

printf 'filter conformance artifacts: %s\n' "$output_dir"
