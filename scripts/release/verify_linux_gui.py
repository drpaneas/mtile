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

from verify_download import desktop_env, explorer_position, unpack


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


def faster_source(source):
    if source.splitlines().count("speed = 2") != 1:
        raise ValueError("expected one Coin Quest speed setting")
    return source.replace("speed = 2", "speed = 3", 1)


def editor_drawn(framebuffer):
    # This private Xvfb screen stores 1920x1200 32-bit pixels after its XWD
    # header. Both the source pane and status bar must have been painted.
    with framebuffer.open("rb") as screen:
        for x, y in ((500, 300), (10, 1190)):
            screen.seek(-((1200 - y) * 1920 - x) * 4, os.SEEK_END)
            if not any(screen.read(3)):
                return False
    return True


def verify(archive, work, display, failure_frame=None, reopen_edited=False):
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
    if reopen_edited:
        subprocess.run([tools / "mtile", "new", "-template", "coin-quest", project],
                       cwd=work, env=env, check=True, timeout=60)
        expected = faster_source(lua.read_text(encoding="utf-8"))
        lua.write_text(expected, encoding="utf-8")

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
            def player_rom():
                found = subprocess.run(["pgrep", "-P", str(editor.pid), "-x", "mtile-player"],
                                       capture_output=True, text=True, timeout=10)
                if found.returncode == 1:
                    return None
                found.check_returncode()
                pid = found.stdout.splitlines()[0]
                args = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
                return Path(os.fsdecode(args[1])) if len(args) > 1 and args[1] else None

            preview = wait_for("Coin Quest player", player_rom, 180)
            window = wait_for("visible Coin Quest editor window", editor_window, 90)
            xdo("windowsize", window, 1920, 1200)
            wait_for("1920x1200 editor window",
                     lambda: "Geometry: 1920x1200" in xdo("getwindowgeometry", window), 10)
            xdo("windowfocus", "--sync", window)
            wait_for("focused, painted editor", lambda: xdo("getwindowfocus", "-f") == window
                     and editor_drawn(work / "Xvfb_screen0"), 60)
            time.sleep(1)
            player = tools / "mtile-player"
            baseline = explorer_position(player, rom, work, env)
            if reopen_edited:
                if preview.resolve() == rom.resolve():
                    raise ValueError("the editor reopened the bundled ROM instead of rebuilding the saved project")
                reopened = explorer_position(player, preview, work, env)
                if reopened[0] != baseline[0] or not reopened[1] > baseline[1]:
                    raise ValueError(f"reopened preview did not use the edited Lua: {baseline} -> {reopened}")
                print(f"Reopened the saved Lua edit in Develop and played its rebuilt preview: {reopened}.", flush=True)
            else:
                source = lua.read_text(encoding="utf-8")
                expected = faster_source(source)
                # Click beyond the speed setting, select its final digit, and replace it.
                line_y = 165 + 20 * source.splitlines().index("speed = 2")
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
                    print(f"Attempt {attempt + 1}: waiting for the GUI save.", flush=True)
                else:
                    raise RuntimeError("the Lua file still matches the original after five GUI save attempts")
            for _ in range(5):
                click(window, 1175, 699)  # Export .gba at 1920x1200
                time.sleep(1)
                if any(project.glob("export-*")):
                    break
            else:
                raise RuntimeError("the Export .gba button did not start an export")
            exported = wait_for("exported cartridge",
                                lambda: next(project.glob("export-*/game.gba"), None), 600)
            edited = explorer_position(player, exported, work, env)
            if edited[0] != baseline[0] or not edited[1] > baseline[1] > baseline[0]:
                raise ValueError(f"GUI export did not move faster: {baseline} -> {edited}")
            print(f"Develop exported speed 3 as {exported.stat().st_size} bytes. "
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
    parser.add_argument("--reopen-edited", action="store_true",
                        help="open an existing Lua edit in Develop and verify its rebuilt preview and GUI export")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="mtile-gui-test-") as temp, private_display(Path(temp)) as display:
        verify(args.archive.resolve(), Path(temp), display, args.failure_frame, args.reopen_edited)
