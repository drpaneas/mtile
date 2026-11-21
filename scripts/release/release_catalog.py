"""Coverage required for desktop release ROM reproducibility."""

EXAMPLES = (
    "audio-gb-melody", "audio-gb-song", "audio-only", "background-layers", "coin-block",
    "font-demo", "hello", "imported-audio", "lantern-rooms", "lumen", "mapdemo", "moon-door",
    "moth-study", "multi-file", "nerdo", "pong", "save-game", "shared-tile-animation",
    "tidewalk", "tile-links",
)
TARGETS = ("darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64")
SHARDS = 4
ATTEMPTS = 3
