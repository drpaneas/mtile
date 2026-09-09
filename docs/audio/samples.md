# PCM samples — deferred

PCM samples and Direct Sound FIFO A/B are intentionally not implemented or
exposed by Audio v2. The release uses only CH1–CH4 so the GBA hardware produces
sound with low, predictable CPU cost.

The superseded sample conversion and software-mixer experiments have been
removed. PCM is not an active asset format, API, or editor screen.

A future PCM feature requires a separate approved plan and incompatible schema
version. Do not add `samples/`, `sample()`, FIFO routing, or software voice
controls to a v2 project.
