# Effective mTile teaching snapshots

These are existing measurements used in the beginner chapter, not new
performance experiments. `measurements.json` retains the selected two-capture
summaries and source-file hashes from commit `b99eb51`. The complete source
report and native evidence are in that commit under
`docs/benchmarks/final-map-palette-2026-10-31/`.

All headline rows here describe complete Mode 0 work at the same 30 FPS
target, two 600-update captures after sixty warm-ups, pinned mGBA and matching
pixels/raw LCD scans/scalar state. Butano is pinned at
`04374b034c76b82bfdc3aa6c38d652e5e7778a68`; the listed control is its strongest
tested ARM/IWRAM caller. Physical hardware confirmation remains outstanding.

The storage slider is a separate 12-byte/4-byte element model, excluding
headers. Neither its bars nor the Lua teaching programs are assigned these
benchmark timings. The chapter's named programs are executed by
`TestEffectiveMtilePrograms`; advanced eligibility is additionally checked
against a reconstructed copy of the optimized revision.
