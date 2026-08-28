# PCM samples — deferred

PCM samples and Direct Sound FIFO A/B are intentionally not implemented or
exposed by Audio v2. The release uses only CH1–CH4 so the GBA hardware produces
sound with low, predictable CPU cost.

Historical sample conversion, mixer, and FIFO experiments remain under
[`legacy/v1`](legacy/v1/README.md) and in reference tooling. They are not an
active asset format, API, editor screen, or completion gate.

A future PCM feature requires a separate approved plan and incompatible schema
version. Do not add `samples/`, `sample()`, FIFO routing, or software voice
controls to a v2 project.
