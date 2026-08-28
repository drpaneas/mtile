#!/usr/bin/env python3
"""Run the PSG coexistence ROM in mGBA and validate its EWRAM telemetry."""

from __future__ import annotations

import argparse
import socket
import subprocess
import time
from pathlib import Path


PASS = 0x50534702
SERVICE_BUDGET = 16384


def symbol_address(map_path: Path, name: str) -> int:
    for line in map_path.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if fields and fields[-1] == name:
            return int(fields[0], 16)
    raise ValueError(f"missing {name} in {map_path}")


class RSP:
    def __init__(self, connection: socket.socket):
        self.connection = connection

    def _packet(self) -> bytes:
        while self.connection.recv(1) != b"$":
            pass
        payload = bytearray()
        while True:
            value = self.connection.recv(1)
            if value == b"#":
                break
            if not value:
                raise ConnectionError("mGBA closed the debugger connection")
            payload.extend(value)
        checksum = self.connection.recv(2)
        expected = sum(payload) & 0xFF
        if int(checksum, 16) != expected:
            self.connection.sendall(b"-")
            raise ValueError("bad GDB remote checksum")
        self.connection.sendall(b"+")
        return bytes(payload)

    def command(self, command: str, response: bool = True) -> bytes:
        payload = command.encode("ascii")
        checksum = sum(payload) & 0xFF
        self.connection.sendall(b"$" + payload + b"#" + f"{checksum:02x}".encode("ascii"))
        acknowledgement = self.connection.recv(1)
        if acknowledgement != b"+":
            raise ConnectionError(f"mGBA rejected debugger command {command!r}")
        return self._packet() if response else b""

    def read_words(self, address: int, count: int) -> list[int]:
        payload = self.command(f"m{address:x},{count * 4:x}")
        if payload.startswith(b"E"):
            raise RuntimeError(f"mGBA memory read failed: {payload.decode()}")
        raw = bytes.fromhex(payload.decode("ascii"))
        return [int.from_bytes(raw[index : index + 4], "little") for index in range(0, len(raw), 4)]


def connect_debugger(deadline: float) -> socket.socket:
    error: OSError | None = None
    while time.monotonic() < deadline:
        try:
            return socket.create_connection(("127.0.0.1", 2345), timeout=2)
        except OSError as current:
            error = current
            time.sleep(0.1)
    raise ConnectionError(f"mGBA debugger did not open port 2345: {error}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mgba", type=Path, required=True)
    parser.add_argument("--rom", type=Path, required=True)
    parser.add_argument("--map", dest="map_path", type=Path, required=True)
    parser.add_argument("--seconds", type=float, default=10.0)
    parser.add_argument("--mode", choices=("paced", "unthrottled"), default="paced")
    parser.add_argument("--fps-target", type=float, default=60.0)
    parser.add_argument("--minimum-service-calls", type=int, default=2048)
    args = parser.parse_args()

    if args.seconds <= 0 or args.fps_target <= 0 or args.minimum_service_calls < 2048:
        parser.error("seconds/fps-target must be positive and minimum-service-calls must be at least 2048")

    names = [
        "main.psgCoexistStatus",
        "main.psgCoexistMaxServiceCycles",
        "main.psgCoexistEvents",
        "main.psgCoexistFrameSkips",
        "main.psgCoexistServiceCalls",
        "main.psgCoexistMinServiceCycles",
        "main.psgCoexistMeanServiceCycles",
        "main.psgCoexistSeenHardware",
        "main.psgCoexistQueueOverflows",
        "main.psgCoexistCommandsRejected",
        "main.psgCoexistFailureFlags",
        "main.psgCoexistFirstFailureService",
    ]
    addresses = [symbol_address(args.map_path, name) for name in names]
    if addresses != list(range(addresses[0], addresses[0] + len(names) * 4, 4)):
        raise ValueError("PSG telemetry words are not contiguous")

    command = [
        str(args.mgba),
        "-g",
        "-C",
        f"audioSync={1 if args.mode == 'paced' else 0}",
        "-C",
        "videoSync=0",
        "-C",
        f"fpsTarget={args.fps_target:g}",
    ]
    command.append(str(args.rom))
    process = subprocess.Popen(
        command,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        connection = connect_debugger(time.monotonic() + 5)
        with connection:
            connection.settimeout(5)
            remote = RSP(connection)
            remote.command("?", response=True)
            remote.command("c", response=False)
            time.sleep(args.seconds)
            connection.sendall(b"\x03")
            remote._packet()
            (
                status,
                max_cycles,
                events,
                frame_skips,
                service_calls,
                min_cycles,
                mean_cycles,
                seen_hardware,
                queue_overflows,
                commands_rejected,
                failure_flags,
                first_failure_service,
            ) = remote.read_words(addresses[0], len(names))
        if status != PASS:
            raise RuntimeError(
                f"PSG coexistence status is 0x{status:08x}, want 0x{PASS:08x}; "
                f"service calls={service_calls}, cycles={min_cycles}/{mean_cycles}/{max_cycles}, "
                f"events={events}, frame skips={frame_skips}, hardware mask=0x{seen_hardware:02x}, "
                f"queue overflows={queue_overflows}, rejected commands={commands_rejected}, "
                f"failure flags=0x{failure_flags:02x}, first failure service={first_failure_service}"
            )
        if max_cycles == 0 or max_cycles > SERVICE_BUDGET:
            raise RuntimeError(f"maximum service cost is {max_cycles} cycles, budget {SERVICE_BUDGET}")
        if events < 64:
            raise RuntimeError(f"only {events} PSG events were applied")
        if service_calls < args.minimum_service_calls:
            raise RuntimeError(
                f"only {service_calls} scheduler calls completed, "
                f"want at least {args.minimum_service_calls}"
            )
        if frame_skips != 0:
            raise RuntimeError(f"graphics coexistence recorded {frame_skips} frame skips")
        if queue_overflows != 0 or commands_rejected != 0:
            raise RuntimeError(
                f"scheduler reported queue overflows={queue_overflows}, "
                f"rejected commands={commands_rejected}"
            )
        if failure_flags != 0:
            raise RuntimeError(f"coexistence failure latch is 0x{failure_flags:02x}")
        if seen_hardware != 0x0F:
            raise RuntimeError(
                f"hardware-active mask is 0x{seen_hardware:02x}, want all CH1-CH4 (0x0f)"
            )
        print("PSG COEXISTENCE: PASS")
        print(f"  256 Hz service min/mean/max: {min_cycles}/{mean_cycles}/{max_cycles} cycles")
        print(f"  maximum slot share: {max_cycles / 65536 * 100:.2f}%")
        print(f"  measured mean CPU share: {mean_cycles / 65536 * 100:.3f}%")
        print(f"  applied events: {events}")
        print(f"  scheduler service calls: {service_calls}")
        print(f"  emulated scheduler time: {service_calls / 256:.2f} seconds")
        print(f"  runner mode: {args.mode}")
        print(f"  graphics frame skips: {frame_skips}")
        print(f"  queue overflows / rejected commands: {queue_overflows}/{commands_rejected}")
        print(f"  failure latch: 0x{failure_flags:02x}")
        print(f"  first failure service: {first_failure_service}")
        print(f"  hardware channels observed: 0x{seen_hardware:02x} (CH1-CH4)")
    finally:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


if __name__ == "__main__":
    main()
