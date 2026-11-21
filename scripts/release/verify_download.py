"""Exercise the extracted desktop download with the user's own project and empty caches."""

import argparse
from contextlib import contextmanager
import hashlib
import os
from pathlib import Path
import plistlib
import shutil
import signal
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
import zipfile


def unpack(archive, destination):
    checksum = Path(str(archive) + ".sha256").read_text().split()[0]
    with archive.open("rb") as stream:
        actual = hashlib.file_digest(stream, "sha256").hexdigest()
    if actual != checksum:
        raise ValueError(f"archive checksum differs: {actual}")
    if sys.platform == "darwin":
        subprocess.run(["/usr/bin/ditto", "-x", "-k", archive, destination], check=True)
        app = destination / "Mtile.app" / "Contents"
        with (app / "Info.plist").open("rb") as stream:
            executable = plistlib.load(stream)["CFBundleExecutable"]
        tools = app / "MacOS"
        return tools / executable, tools, app / "Resources" / "coin-quest.gba"
    if os.name == "nt":
        with zipfile.ZipFile(archive) as download:
            download.extractall(destination)
        root = destination / archive.stem
        return root / "Mtile.exe", root / "bin", root / "coin-quest.gba"
    with tarfile.open(archive, "r:gz") as download:
        download.extractall(destination, filter="data")
    root = destination / archive.name.removesuffix(".tar.gz")
    return root / "Mtile", root / "bin", root / "coin-quest.gba"


def desktop_env(home):
    env = {key: value for key, value in os.environ.items()
           if key in {"DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "WAYLAND_DISPLAY", "LANG",
                      "COMSPEC", "PATHEXT", "WINDIR", "SYSTEMROOT", "SystemRoot", "NUMBER_OF_PROCESSORS"}}
    env.update(HOME=str(home), USERPROFILE=str(home), PATH=str(home / "no-developer-tools"),
               APPDATA=str(home / "roaming"), LOCALAPPDATA=str(home / "local"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_CONFIG_HOME=str(home / "config"),
               GOPATH=str(home / "gopath"), GOMODCACHE=str(home / "modules"),
               GOCACHE=str(home / "gocache"), GOENV="off", GOPROXY="off", GOSUMDB="off",
               GOTOOLCHAIN="local", GOTELEMETRY="off", TMPDIR=str(home / "tmp"),
               TEMP=str(home / "tmp"), TMP=str(home / "tmp"))
    for name in ("no-developer-tools", "tmp", "local", "roaming"):
        (home / name).mkdir(parents=True)
    return env


@contextmanager
def windows_offline(install, enabled):
    if not enabled:
        yield
        return
    if os.name != "nt":
        raise ValueError("--block-windows-network requires Windows")
    # Restrict the extracted programs. The Actions runner keeps its own connection.
    env = dict(os.environ, MTILE_TEST_INSTALL=str(install), MTILE_FIREWALL_GROUP=install.parent.name)
    shell = ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command"]
    add = """
$ErrorActionPreference = 'Stop'
if (Get-NetFirewallProfile | Where-Object { -not $_.Enabled }) {
    throw 'The offline test requires Windows Firewall to be enabled'
}
Get-ChildItem -LiteralPath $env:MTILE_TEST_INSTALL -Recurse -File -Filter '*.exe' | ForEach-Object {
    New-NetFirewallRule -DisplayName $_.Name -Group $env:MTILE_FIREWALL_GROUP -Program $_.FullName -Direction Outbound -Action Block -Profile Any | Out-Null
}
"""
    remove = "Get-NetFirewallRule -Group $env:MTILE_FIREWALL_GROUP -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction Stop"
    try:
        subprocess.run(shell + [add], env=env, check=True, timeout=180)
        yield
    finally:
        subprocess.run(shell + [remove], env=env, check=True, timeout=90)


def player_started(editor):
    if os.name == "nt":
        query = f"Get-Process -Name mtile-player -ErrorAction SilentlyContinue | Where-Object {{ (Get-CimInstance Win32_Process -Filter ('ProcessId=' + $_.Id)).ParentProcessId -eq {editor.pid} }} | Select-Object -ExpandProperty Id"
        result = subprocess.run(["powershell.exe", "-NoProfile", "-Command", query],
                                capture_output=True, text=True, timeout=30)
        return bool(result.stdout.strip())
    # macOS ps is setuid and cannot be executed in the network-denied sandbox.
    result = subprocess.run(["pgrep", "-P", str(editor.pid), "-x", "mtile-player"],
                            capture_output=True, text=True, timeout=10)
    if result.returncode not in (0, 1):
        raise RuntimeError(f"cannot check the editor's child process: {result.stderr}")
    return result.returncode == 0


def hello_window(cli):
    title = "mtile editor - hello"
    if os.name == "nt":
        import ctypes
        user32 = ctypes.windll.user32
        user32.FindWindowW.restype = ctypes.c_void_p
        window = user32.FindWindowW(None, title)
        return window and user32.IsWindowVisible(ctypes.c_void_p(window))
    if sys.platform == "darwin":
        # macOS can hide window titles without Screen Recording permission.
        # Check the on-screen window of the CLI's actual Hello editor process.
        child = subprocess.run(["pgrep", "-P", str(cli.pid), "-f", "mtile-editor -project .*/examples/hello$"],
                               capture_output=True, text=True, timeout=10)
        if child.returncode == 1:
            return False
        child.check_returncode()
        script = """
        ObjC.import("CoreGraphics")
        function run(argv) {
            const info = $.CGWindowListCopyWindowInfo($.kCGWindowListOptionOnScreenOnly, 0)
            const windows = ObjC.deepUnwrap(ObjC.castRefToObject(info))
            return windows.some(w => w.kCGWindowOwnerPID == argv[0] && w.kCGWindowLayer == 0 &&
                                w.kCGWindowBounds.Width > 100 && w.kCGWindowBounds.Height > 100)
        }
        """
        visible = subprocess.check_output(["osascript", "-l", "JavaScript", "-e", script, child.stdout.strip()],
                                          text=True, timeout=15)
        return visible.strip() == "true"
    # The Linux release workflow provides a private X display and xdotool.
    result = subprocess.run(["xdotool", "search", "--onlyvisible", "--name", "^" + title + "$"],
                            capture_output=True, text=True, timeout=10)
    if result.returncode not in (0, 1):
        raise RuntimeError(f"cannot inspect the editor window: {result.stderr}")
    return bool(result.stdout.strip())


def open_and_wait(command, work, env, ready, label):
    with tempfile.TemporaryFile() as output:
        editor = subprocess.Popen(command, cwd=work, env=env, stdout=output, stderr=output,
                                  start_new_session=os.name != "nt")
        try:
            deadline = time.monotonic() + 90
            while editor.poll() is None and time.monotonic() < deadline:
                if ready(editor):
                    return
                time.sleep(0.3)
            output.seek(0)
            raise RuntimeError(f"desktop startup failed to open {label}: " +
                               output.read().decode(errors="replace"))
        finally:
            if editor.poll() is None:
                if os.name == "nt":
                    subprocess.run(["taskkill", "/F", "/T", "/PID", str(editor.pid)],
                                   capture_output=True, timeout=30)
                else:
                    os.killpg(editor.pid, signal.SIGTERM)
                editor.wait(timeout=20)


def explorer_position(player, rom, work, env):
    # Each draw fills the rows behind the 8x8 explorer with a solid color.
    # Stop well before the first coin. Measure the leftmost visible sprite pixel.
    commands = bytes([1, 0, 0, 0]) * 75 + bytes([1, 0, 16, 0]) * 16 + bytes([3, 0, 0, 0])
    packets = subprocess.run([player, rom], input=commands, capture_output=True, cwd=work,
                             env=env, check=True, timeout=60).stdout
    magic, _, width, height, _, _ = struct.unpack_from("<4sHHHHI", packets)
    if (magic, width, height) != (b"GLUP", 240, 160):
        raise ValueError("invalid mGBA handshake")
    offset = 16
    positions = []
    for tick in range(91):
        magic, number, samples, overflow, telemetry = struct.unpack_from("<4sIIII", packets, offset)
        if (magic, number, overflow) != (b"FRAM", tick, 0):
            raise ValueError(f"bad mGBA response at frame {tick}")
        offset += 20
        if tick in (74, 90):
            pixels = memoryview(packets)[offset:offset + width * height * 4].cast("I")
            positions.append(min(x for y in range(72, 80) for x in range(8, 100)
                                 if pixels[y * width + x] != pixels[y * width]))
        offset += width * height * 4 + samples * 4 + telemetry
    if offset != len(packets):
        raise ValueError("unexpected trailing emulator output")
    return positions


def verify(archive, launch, block_windows_network):
    with tempfile.TemporaryDirectory(prefix="mtile-release-test-") as tmp:
        work = Path(tmp)
        install = work / "Moved Desktop Download"
        install.mkdir()
        launcher, tools, starter_rom = unpack(archive.resolve(), install)
        extension = ".exe" if os.name == "nt" else ""
        cli = tools / ("mtile" + extension)
        player = tools / ("mtile-player" + extension)
        home = work / "New User"
        env = desktop_env(home)
        project = home / "mTile Projects" / "Coin Quest"
        with windows_offline(install, block_windows_network):
            if launch:
                open_and_wait([launcher], work, env, player_started, "the Coin Quest player")
                print("No-argument desktop launcher opened the Coin Quest player.", flush=True)
            else:
                subprocess.run([cli, "new", "-template", "coin-quest", project], cwd=work,
                               env=env, check=True, timeout=60)
            subprocess.run([cli, "doctor"], cwd=work, env=env, check=True, timeout=60)
            baseline = explorer_position(player, starter_rom, work, env)
            lua = project / "game.lua"
            original = lua.read_text(encoding="utf-8")
            if original.count("speed = 2") != 1:
                raise ValueError("expected the Coin Quest starter movement speed")
            lua.write_text(original.replace("speed = 2", "speed = 3"), encoding="utf-8")
            export = project / "my faster game.gba"
            subprocess.run([cli, "build", "-o", export, project], cwd=work, env=env,
                           check=True, timeout=600)
            edited = explorer_position(player, export, work, env)
            if edited[0] != baseline[0] or not edited[1] > baseline[1] > baseline[0]:
                raise ValueError(f"the rebuilt game did not move faster: {baseline} -> {edited}")
            print(f"Right for 16 frames: original x={baseline}, edited x={edited}. "
                  f"Exported {export.stat().st_size} bytes.")

            root = install if sys.platform == "darwin" else launcher.parent
            env["PATH"] = str(tools) + os.pathsep + env["PATH"]
            if shutil.which(cli.name, path=env["PATH"]) != str(cli):
                raise RuntimeError("the documented PATH did not select the bundled mtile")
            # Use the guide's PowerShell on Windows. cmd.exe would search the
            # current folder first and launch the double-clickable Mtile.exe.
            command = ["mtile"]
            if os.name == "nt":
                command = [shutil.which("powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", "mtile"]
            example = "examples/hello"
            rom = root / example / "game.gba"
            if not (root / example / "game.lua").is_file() or rom.exists():
                raise ValueError("expected the bundled Hello Lua project without a prebuilt game.gba")
            if launch:
                open_and_wait(command + ["edit", example], root, env, hello_window, "the Hello project window")
                print("mtile edit examples/hello opened the bundled Hello editor window.", flush=True)
            subprocess.run(command + ["build", example], cwd=root, env=env, check=True, timeout=600)
            result = rom.read_bytes()
            if len(result) <= 192 or result[0xB2] != 0x96:
                raise ValueError(f"invalid GBA cartridge built from {example}")
            print(f"mtile build examples/hello created {rom.relative_to(root)} "
                  f"({len(result):,} bytes, SHA-256 {hashlib.sha256(result).hexdigest()}).")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--launch", action="store_true", help="also open the desktop window (requires a display)")
    parser.add_argument("--block-windows-network", action="store_true",
                        help="temporarily block outbound connections for the extracted executables (administrator)")
    args = parser.parse_args()
    verify(args.archive, args.launch, args.block_windows_network)
