"""Play the Linux download, edit Coin Quest in Develop, and export via its GUI."""

import argparse
from contextlib import contextmanager, suppress
from difflib import unified_diff
import os
from pathlib import Path
import select
import shutil
import signal
import subprocess
import tempfile
import time

from verify_download import desktop_env, explorer_position, player_started, unpack


@contextmanager
def private_display(frame_dir):
    # Xvfb chooses a free display and writes its number when it is ready.
    # The editor and xdotool only see this test's server.
    with subprocess.Popen(["Xvfb", "-displayfd", "1", "-screen", "0", "1920x1200x24",
                           "-dpi", "96", "-nolisten", "tcp", "-fbdir", str(frame_dir)], stdout=subprocess.PIPE) as server:
        try:
            if not select.select([server.stdout], [], [], 20)[0]:
                raise RuntimeError("Xvfb did not start")
            number = server.stdout.readline().strip()
            if not number.isdigit():
                raise RuntimeError("Xvfb did not provide a display number")
            yield ":" + number.decode("ascii")
        finally:
            server.terminate()


def verify(archive, work, display, failure_frame=None):
    install = work / "Moved Desktop Download"
    install.mkdir()
    launcher, tools, rom = unpack(archive, install)
    home = work / "New Player"
    env = desktop_env(home)
    env["DISPLAY"] = display
    env.pop("WAYLAND_DISPLAY", None)
    env.pop("XAUTHORITY", None)
    driver_env = dict(env, PATH=os.environ["PATH"])
    project = home / "mTile Projects" / "Coin Quest"
    lua = project / "game.lua"

    def xdo(*args):
        return subprocess.check_output(["xdotool", *map(str, args)], env=driver_env,
                                       text=True, timeout=10).strip()

    def editor_window():
        # The player can start before Ebitengine maps the editor's X11 window.
        # The private display has only this editor, so its title is sufficient
        # even when the X11 client has no _NET_WM_PID property.
        result = subprocess.run(["xdotool", "search", "--onlyvisible", "--name",
                                 "^mtile editor - Coin Quest$"], env=driver_env,
                                capture_output=True, text=True, timeout=10)
        if result.returncode == 1 and not result.stderr:
            return None
        result.check_returncode()
        return next(iter(result.stdout.splitlines()), None)

    def key(name, modifier=None):
        if modifier:
            xdo("keydown", modifier)
            time.sleep(0.2)
        xdo("keydown", name)
        time.sleep(0.13)
        xdo("keyup", name)
        if modifier:
            xdo("keyup", modifier)
        time.sleep(0.15)

    def click(window, x, y):
        xdo("mousemove", "--window", window, x, y)
        time.sleep(0.15)
        xdo("mousedown", "1")
        time.sleep(0.35)
        xdo("mouseup", "1")
        time.sleep(0.2)

    with subprocess.Popen([launcher], cwd=work, env=env, start_new_session=True) as editor:
        def wait_for(label, predicate, timeout):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline and editor.poll() is None:
                result = predicate()
                if result:
                    return result
                time.sleep(0.3)
            windows = subprocess.run(["xdotool", "search", "--onlyvisible", "--name", ".*",
                                      "getwindowname", "%@"], env=driver_env, capture_output=True,
                                     text=True, timeout=10)
            raise RuntimeError(f"waiting for {label}: editor exit={editor.poll()}, "
                               f"visible window titles={windows.stdout.splitlines()}, {windows.stderr.strip()}")

        try:
            wait_for("starter preview", lambda: player_started(editor), 90)
            window = wait_for("visible Coin Quest editor window", editor_window, 90)
            xdo("windowfocus", "--sync", window)
            xdo("windowsize", window, 1920, 1200)
            wait_for("1920x1200 editor window",
                     lambda: "Geometry: 1920x1200" in xdo("getwindowgeometry", window), 10)
            source = lua.read_text(encoding="utf-8")
            lines = source.splitlines()
            if lines.count("speed = 2") != 1:
                raise ValueError("expected one Coin Quest speed setting")
            expected = source.replace("speed = 2", "speed = 3", 1)
            # In the default 1920x1200 Develop layout, the first line starts
            # at y=155 and rows are 20 px tall. Clicking after this short line
            # places the caret at its end regardless of the previous selection.
            line_y = 165 + 20 * lines.index("speed = 2")
            for attempt in range(5):
                click(window, 500, line_y)
                key("Left", "Shift_L")
                key("3")
                key("s", "Control_L")
                time.sleep(1)
                saved = lua.read_text(encoding="utf-8")
                if saved == expected:
                    break
                if saved != source:
                    diff = "".join(unified_diff(expected.splitlines(True), saved.splitlines(True),
                                                fromfile="expected/game.lua", tofile="saved/game.lua"))
                    raise RuntimeError("the editor saved a different Lua edit:\n" + diff)
                print(f"Attempt {attempt + 1}: the editor has not saved the speed edit yet.", flush=True)
            else:
                raise RuntimeError("the Lua file still matches the original after five GUI save attempts")
            click(window, 1175, 699)  # Export .gba at 1920x1200
            exported = wait_for("exported cartridge",
                                lambda: next(project.glob("export-*/game.gba"), None), 600)
            player = tools / "mtile-player"
            baseline = explorer_position(player, rom, work, env)
            edited = explorer_position(player, exported, work, env)
            if edited[0] != baseline[0] or not edited[1] > baseline[1] > baseline[0]:
                raise ValueError(f"GUI export did not move faster: {baseline} -> {edited}")
            print(f"Develop saved speed 3 and exported {exported.stat().st_size} bytes. "
                  f"Right for 16 frames: {baseline} -> {edited}.")
        except Exception:
            frame = work / "Xvfb_screen0"
            if failure_frame and frame.is_file():
                shutil.copyfile(frame, failure_frame)
            raise
        finally:
            # Reap the private session even if an export or background compiler
            # is still writing into the temporary user's cache after a failure.
            with suppress(ProcessLookupError):
                os.killpg(editor.pid, signal.SIGKILL)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--failure-frame", type=Path, help="save the private X11 screen on failure (XWD)")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="mtile-gui-test-") as temp, private_display(Path(temp)) as display:
        verify(args.archive.resolve(), Path(temp), display, args.failure_frame)
