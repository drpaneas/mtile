#!/bin/bash
set -euo pipefail
WORK="${1:?usage: measure-mapdemo.sh <build-workdir>}"
ELF="$WORK/game.elf"
GBA="$WORK/game.gba"

pkill -f "mgba -g" 2>/dev/null || true
sleep 1

/Applications/mGBA.app/Contents/MacOS/mgba -g "$GBA" >/tmp/mgba-mapdemo.log 2>&1 &
MGBA_PID=$!
sleep 3

GDB=/opt/homebrew/bin/gdb

ADDR_OUT=$("$GDB" --batch \
  -ex "print &('main.ctx'.profileFrames)" \
  -ex "print &('main.ctx'.maxFrameCycles)" \
  -ex "print &('main.ctx'.lastFrameCycles)" \
  -ex "print &('main.ctx'.heap.collectCount)" \
  "$ELF" 2>/dev/null)

FRAMES=$(echo "$ADDR_OUT" | sed -n '1p' | grep -o '0x[0-9a-f]*')
MAX=$(echo "$ADDR_OUT" | sed -n '2p' | grep -o '0x[0-9a-f]*')
LAST=$(echo "$ADDR_OUT" | sed -n '3p' | grep -o '0x[0-9a-f]*')
GC=$(echo "$ADDR_OUT" | sed -n '4p' | grep -o '0x[0-9a-f]*')

echo "addrs: frames=$FRAMES max=$MAX last=$LAST gc=$GC"

"$GDB" --batch \
  -ex "set architecture armv4t" \
  -ex "target remote localhost:2345" \
  -ex "continue&" \
  -ex "shell sleep 12" \
  -ex "interrupt" \
  -ex "shell sleep 0.5" \
  -ex "x/1wd $FRAMES" \
  -ex "x/1wd $MAX" \
  -ex "x/1wd $LAST" \
  -ex "x/1wd $GC" \
  "$ELF" 2>&1 | tee /tmp/gdb-mapdemo-12s.txt | grep "0x020\|0x030" || true

echo "--- at first GC (watch collectCount >= 1) ---"
"$GDB" --batch \
  -ex "set architecture armv4t" \
  -ex "target remote localhost:2345" \
  -ex "watch *(unsigned int*)$GC >= 1" \
  -ex "continue" \
  -ex "x/1wd $FRAMES" \
  -ex "x/1wd $MAX" \
  -ex "x/1wd $LAST" \
  -ex "x/1wd $GC" \
  "$ELF" 2>&1 | tee /tmp/gdb-mapdemo-gc.txt | grep -E "Hardware watchpoint|0x020|0x030" || true

kill "$MGBA_PID" 2>/dev/null || true

python3 - <<'PY'
import re
budget = 561792
for label, path in [("12s", "/tmp/gdb-mapdemo-12s.txt"), ("GC", "/tmp/gdb-mapdemo-gc.txt")]:
    try:
        text = open(path).read()
    except FileNotFoundError:
        print(label, "no output")
        continue
    vals = [int(m.group(1)) for m in re.finditer(r'0x[0-9a-f]+:\s+(\d+)', text)]
    if len(vals) >= 4:
        frames, maxc, last, gc = vals[-4], vals[-3], vals[-2], vals[-1]
        print(f"{label}: frames={frames} max={maxc} ({maxc/budget*100:.1f}%) last={last} gcCount={gc}")
    else:
        print(f"{label}: could not parse ({len(vals)} vals)")
        print(text[-800:])
PY
