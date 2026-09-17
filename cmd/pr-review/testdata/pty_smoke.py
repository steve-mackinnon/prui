"""Compiled-binary smoke journeys; invoked by TestPTYSmoke, no pip dependencies."""

import errno
import codecs
import fcntl
import os
import pathlib
import pty
import re
import selectors
import signal
import struct
import subprocess
import sys
import termios
import time


TIMEOUT = 10
MAX_OUTPUT = 2 * 1024 * 1024


def terminate(signum, frame):
    # Unwind active Terminal context managers before exiting on Go's deadline.
    raise SystemExit("terminal test canceled")


signal.signal(signal.SIGTERM, terminate)


class Screen:
    """Small VT screen for these ASCII fixtures, including incremental redraws.

    Stripping ANSI from a transcript is insufficient: the renderer can update
    only the selection marker or one digit. Keep cursor/erase operations so
    expectations refer to the screen the user sees, not old output bytes.
    """

    def __init__(self):
        self.row = self.col = 0
        self.pending = ""
        self.decoder = codecs.getincrementaldecoder("utf-8")()
        self.cells = []
        self.resize(120, 24)

    def resize(self, width, height):
        self.width, self.height = width, height
        self.cells = [(row[:width] + [" "] * width)[:width] for row in self.cells[:height]]
        self.cells.extend([[" "] * width for _ in range(height - len(self.cells))])
        self.row, self.col = min(self.row, height - 1), min(self.col, width - 1)

    def linefeed(self):
        self.row += 1
        if self.row >= self.height:
            self.cells.pop(0)
            self.cells.append([" "] * self.width)
            self.row = self.height - 1

    def csi(self, params, final):
        if any(c not in "0123456789;" for c in params):
            return  # private terminal modes, capability queries, attributes
        values = [int(v or 0) for v in params.split(";")]
        n = values[0] or 1
        if final in "Hf":
            self.row = n - 1
            self.col = (values[1] or 1) - 1 if len(values) > 1 else 0
        elif final == "A": self.row -= n
        elif final == "B": self.row += n
        elif final == "C": self.col += n
        elif final == "D": self.col -= n
        elif final in "G`": self.col = n - 1
        elif final == "d": self.row = n - 1
        elif final == "J":
            mode = values[0]
            if mode in (2, 3): self.cells = [[" "] * self.width for _ in range(self.height)]
            elif mode == 0:
                self.cells[self.row][self.col:] = [" "] * (self.width - self.col)
                for i in range(self.row + 1, self.height): self.cells[i] = [" "] * self.width
            elif mode == 1:
                for i in range(self.row): self.cells[i] = [" "] * self.width
                self.cells[self.row][:self.col + 1] = [" "] * (self.col + 1)
        elif final == "K":
            mode = values[0]
            start, end = (0, self.width) if mode == 2 else ((0, self.col + 1) if mode == 1 else (self.col, self.width))
            self.cells[self.row][start:end] = [" "] * (end - start)
        elif final == "X":
            end = min(self.width, self.col + n)
            self.cells[self.row][self.col:end] = [" "] * (end - self.col)
        elif final not in "mhlnc":
            raise AssertionError(f"unsupported screen operation: CSI {params}{final}")
        self.row = max(0, min(self.row, self.height - 1))
        self.col = max(0, min(self.col, self.width - 1))

    def feed(self, data):
        self.pending += self.decoder.decode(data)
        while self.pending:
            if self.pending.startswith("\x1b["):
                match = re.match(r"\x1b\[([0-?]*[ -/]*)([@-~])", self.pending)
                if match is None: return
                self.csi(*match.groups())
                self.pending = self.pending[match.end():]
                continue
            if self.pending.startswith("\x1b]"):
                match = re.match(r"\x1b\].*?(?:\x07|\x1b\\)", self.pending, re.S)
                if match is None: return
                self.pending = self.pending[match.end():]
                continue
            char = self.pending[0]
            if char == "\x1b":
                if len(self.pending) < 2: return
                if self.pending[1] == "M":
                    if self.row == 0:
                        self.cells.insert(0, [" "] * self.width)
                        self.cells.pop()
                    else:
                        self.row -= 1
                    self.pending = self.pending[2:]
                    continue
                raise AssertionError(f"unsupported terminal escape: {self.pending[:8]!r}")
            self.pending = self.pending[1:]
            if char == "\r": self.col = 0
            elif char == "\n": self.linefeed()
            elif char == "\b": self.col = max(0, self.col - 1)
            elif char == "\t": self.col = min(self.width - 1, (self.col // 8 + 1) * 8)
            elif char >= " ":
                if self.col >= self.width:
                    self.col = 0
                    self.linefeed()
                self.cells[self.row][self.col] = char
                self.col += 1

    def text(self):
        return "\n".join("".join(row).rstrip() for row in self.cells)


class Terminal:
    def __init__(self, binary, args, environment):
        self.master, self.slave = pty.openpty()
        self.original = termios.tcgetattr(self.slave)
        self.output = bytearray()
        self.screen = Screen()
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.master, selectors.EVENT_READ)
        self.resize(120, 24, notify=False)

        # Isolate the process group without making the child the controlling
        # terminal's session leader: macOS revokes that PTY when its leader exits,
        # preventing inspection of restored termios through the retained slave.
        self.process = subprocess.Popen(
            [binary, *args], stdin=self.slave, stdout=self.slave,
            stderr=self.slave, env=environment, start_new_session=True,
        )

    def __enter__(self):
        return self

    def __exit__(self, kind, error, traceback):
        if self.process.poll() is None:
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait(timeout=TIMEOUT)
        self.selector.close()
        os.close(self.master)
        os.close(self.slave)

    def resize(self, width, height, notify=True):
        self.screen.resize(width, height)
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        if notify:
            os.kill(self.process.pid, signal.SIGWINCH)

    def read(self, timeout):
        if self.selector.select(timeout):
            try:
                data = os.read(self.master, 65536)
                if not data:
                    return False
                self.output.extend(data)
                self.screen.feed(data)
                if len(self.output) > MAX_OUTPUT:
                    self.fail("terminal output exceeded 2 MiB")
                return True
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
        return False

    def fail(self, message):
        raise AssertionError(f"{message}; exit={self.process.poll()}\nScreen:\n{self.screen.text()}\nPTY tail: {bytes(self.output[-12000:])!r}")

    def wait_until(self, predicate, label, since=0):
        deadline = time.monotonic() + TIMEOUT
        while True:
            if len(self.output) > since and predicate(self.screen.text()):
                return
            if self.process.poll() is not None:
                self.read(0)
                self.fail(f"exited waiting for {label!r}")
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                self.fail(f"timed out waiting for {label!r}")
            self.read(min(remaining, 0.1))

    def wait_for(self, text, since=0):
        self.wait_until(lambda screen: text in screen, text, since)

    def key(self, value, expected):
        start = len(self.output)
        os.write(self.master, value)
        self.wait_for(expected, start)

    def quit(self, key):
        os.write(self.master, key)
        deadline = time.monotonic() + TIMEOUT
        while self.process.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                self.fail("quit did not exit")
            self.read(min(remaining, 0.1))
        # Drain the final renderer shutdown bytes without waiting for EOF:
        # the parent deliberately keeps the slave open to inspect termios.
        while self.selector.select(0):
            if not self.read(0):
                break
        if self.process.returncode != 0:
            self.fail("nonzero exit after quit")
        restored = termios.tcgetattr(self.slave)
        # PENDIN is the kernel's transient "retype pending input" state, set on
        # macOS when canonical mode is restored. Compare every configured mode
        # and control character, excluding only that pending-input indicator.
        expected = list(self.original)
        expected[3] &= ~getattr(termios, "PENDIN", 0)
        restored[3] &= ~getattr(termios, "PENDIN", 0)
        if restored != expected:
            self.fail(f"terminal modes were not restored: before={self.original!r}, after={restored!r}")
        for sequence in (b"\x1b[?1049h", b"\x1b[?1049l", b"\x1b[?25h"):
            if sequence not in self.output:
                self.fail(f"missing terminal setup/restoration sequence {sequence!r}")


def main():
    # Verify that split escape sequences and marker-only redraws reconstruct
    # the same screen, and erased text cannot satisfy a later expectation.
    trace = b"\x1b[H\x1b[2Jheading\r\n> alpha\r\n  beta\r\x1b[2d  \r\n> "
    for split in range(len(trace) + 1):
        screen = Screen()
        screen.feed(trace[:split])
        screen.feed(trace[split:])
        if screen.text().splitlines()[:3] != ["heading", "  alpha", "> beta"]:
            raise AssertionError("incremental screen decoding failed")
        screen.feed(b"\x1b[H\x1b[2Jempty")
        if "alpha" in screen.text() or "beta" in screen.text():
            raise AssertionError("erased content remained visible")

    binary, store, session_id = sys.argv[1:]
    root = pathlib.Path(store).parent
    bin_dir = root / "fake-bin"
    bin_dir.mkdir()
    gh_called = root / "gh-called"
    # Any unexpected GitHub invocation fails locally and leaves evidence. No
    # inherited credentials, user configuration, PATH executables, or endpoints.
    gh = bin_dir / "gh"
    gh.write_text('#!/bin/sh\nprintf called > "$HOME/gh-called"\nexit 97\n')
    gh.chmod(0o700)
    environment = {
        "TERM": "xterm-256color", "PATH": str(bin_dir), "HOME": str(root),
        "XDG_CONFIG_HOME": str(root), "GH_CONFIG_DIR": str(root), "LC_ALL": "C",
    }
    with Terminal(binary, ["prs", "--store", store], environment) as terminal:
        terminal.wait_for("Remembered repositories")
        terminal.key(b"\x1b[B", "> owner/beta")
        terminal.key(b"\x1b[A", "> owner/alpha")
        terminal.quit(b"q")
    print("PASS browser startup, arrow keys, q, terminal restoration")

    resume = ["resume", session_id, "--store", store, "--offline"]
    with Terminal(binary, resume, environment) as terminal:
        terminal.wait_for("0/2 read (local)")
        terminal.key(b"\x1b[B", "unit 2/2")
        terminal.key(b"m", "1/2 read (local)")
        # Resizing across the split-pane breakpoint must cause a real repaint.
        start = len(terminal.output)
        terminal.resize(70, 12)
        terminal.wait_until(lambda screen: "> [x] b.go" in screen and
                            all(" | " not in row for row in screen.splitlines()[3:10]),
                            "narrow file pane after resize", start)
        terminal.key(b"?", "Keyboard")
        terminal.key(b"\x1b", "1/2 read (local)")
        terminal.quit(b"\x03")
    print("PASS resume, keyboard marking, resize, help/back, Ctrl+C, restoration")

    with Terminal(binary, resume, environment) as terminal:
        terminal.wait_for("1/2 read (local)")
        terminal.wait_for("[x] b.go")
        terminal.quit(b"q")
    if gh_called.exists():
        raise AssertionError("browser startup/offline journeys unexpectedly invoked GitHub")
    print("PASS restart/resume preserves marked file without GitHub calls")


if __name__ == "__main__":
    main()
