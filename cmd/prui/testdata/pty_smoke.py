"""Compiled-binary smoke journeys; invoked by TestPTYSmoke, no pip dependencies."""

import base64
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
        self.scroll_top, self.scroll_bottom = 0, height - 1

    def scroll(self, top, bottom, count):
        count = max(-(bottom - top + 1), min(count, bottom - top + 1))
        for _ in range(abs(count)):
            if count > 0:
                self.cells.pop(top)
                self.cells.insert(bottom, [" "] * self.width)
            else:
                self.cells.pop(bottom)
                self.cells.insert(top, [" "] * self.width)

    def linefeed(self):
        if self.row == self.scroll_bottom:
            self.scroll(self.scroll_top, self.scroll_bottom, 1)
        else:
            self.row = min(self.height - 1, self.row + 1)

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
        elif final == "r":
            top = n - 1
            bottom = (values[1] or self.height) - 1 if len(values) > 1 else self.height - 1
            if 0 <= top < bottom < self.height:
                self.scroll_top, self.scroll_bottom = top, bottom
                self.row = self.col = 0
        elif final == "S": self.scroll(self.scroll_top, self.scroll_bottom, n)
        elif final == "T": self.scroll(self.scroll_top, self.scroll_bottom, -n)
        elif final in "LM":
            if self.scroll_top <= self.row <= self.scroll_bottom:
                self.scroll(self.row, self.scroll_bottom, n if final == "M" else -n)
        elif final in "P@":
            count = min(n, self.width - self.col)
            row = self.cells[self.row]
            if final == "P":
                row[self.col:] = row[self.col + count:] + [" "] * count
            else:
                row[self.col:] = ([" "] * count + row[self.col:])[:self.width - self.col]
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
                    if self.row == self.scroll_top:
                        self.scroll(self.scroll_top, self.scroll_bottom, -1)
                    else:
                        self.row = max(0, self.row - 1)
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
        for sequence in (b"\x1b[?1049h", b"\x1b[?1049l", b"\x1b[?25h",
                         b"\x1b[?1002h", b"\x1b[?1002l"):
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

    # Scrolling a region must retain the header/footer outside that region.
    region_trace = b"\x1b[H\x1b[2Jheader\r\nfirst\r\nsecond\r\nfooter\x1b[2;3r\x1b[3;1H\n\x1b[2;1H\x1bM"
    for split in range(len(region_trace) + 1):
        screen = Screen()
        screen.feed(region_trace[:split])
        screen.feed(region_trace[split:])
        if screen.text().splitlines()[:4] != ["header", "", "second", "footer"]:
            raise AssertionError("incremental scroll-region decoding failed")

    character_trace = b"\x1b[H\x1b[2Jabcdef\x1b[1;3H\x1b[2P\x1b[@Z"
    for split in range(len(character_trace) + 1):
        screen = Screen()
        screen.feed(character_trace[:split])
        screen.feed(character_trace[split:])
        if screen.text().splitlines()[0] != "abZef":
            raise AssertionError("incremental character insert/delete decoding failed")

    binary, home, session_id = sys.argv[1:]
    root = pathlib.Path(home)
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
        "XDG_CONFIG_HOME": str(root), "XDG_DATA_HOME": str(root), "GH_CONFIG_DIR": str(root), "LC_ALL": "C",
    }
    with Terminal(binary, ["prs"], environment) as terminal:
        terminal.wait_for("Repositories")
        terminal.key(b"\x1b[B", "› owner/beta")
        terminal.key(b"\x1b[A", "› owner/alpha")
        terminal.quit(b"q")
    print("PASS browser startup, arrow keys, q, terminal restoration")

    resume = ["resume", session_id, "--offline"]
    with Terminal(binary, resume, environment) as terminal:
        terminal.wait_for("0/2 read")
        terminal.wait_for("› Overview [1]")
        terminal.key(b"2", "› Files [2]")
        terminal.key(b"C", "Select commits for one net diff.")
        terminal.wait_for("╭")
        terminal.wait_for("› Files [2]")
        terminal.key(b"j", "› [ ] cccccccccccc")
        terminal.key(b"\r", "1 commits")
        start = len(terminal.output)
        os.write(terminal.master, b"\x1b")
        terminal.wait_until(lambda screen: "╭" not in screen and
                            "Selected commits unavailable" in screen,
                            "close commit modal", start)
        terminal.wait_for("reading only")
        terminal.key(b"C", "╭")
        terminal.wait_for("1 commits")
        terminal.key(b"\x1b[H", "› [ ] All commits")
        terminal.key(b"\r", "All commits")
        start = len(terminal.output)
        os.write(terminal.master, b"\x1b")
        terminal.wait_until(lambda screen: "╭" not in screen and "0/2 read" in screen,
                            "restore review after commit modal", start)
        terminal.key(b"F", "Filter files:")
        terminal.key(b"\x1b[200~b.go\x1b[201~", "Filter files: b.go")
        start = len(terminal.output)
        os.write(terminal.master, b"\x1b")
        terminal.wait_until(lambda screen: "Filter files:" not in screen and
                            "› [ ] b.go" in screen,
                            "clear pasted file filter", start)
        terminal.key(b"/", "Type to search diff text")
        terminal.key(b"needle", "2 matches in 2 files")
        terminal.key(b"q", "No matches in saved diff text")
        terminal.key(b"\x7f", "2 matches in 2 files")
        terminal.key(b"\x1b", "j/k")
        terminal.key(b"j", "›  R1 + search-needle-b.go")
        start = len(terminal.output)
        os.write(terminal.master, b"\r")
        terminal.wait_until(lambda screen: "Find: needle" not in screen and
                            "› [ ] b.go" in screen and "search-needle-b.go" in screen,
                            "search result activation", start)
        terminal.key(b"h", "› [ ] b.go")
        terminal.key(b"\x1b[A", "› [ ] a.go")
        # SGR coordinates are one-based. Select the second file without an
        # activation key, then drag the existing divider from column 38 to 50.
        terminal.key(b"\x1b[<0;5;5M\x1b[<0;5;5m", "› [ ] b.go")
        start = len(terminal.output)
        os.write(terminal.master, b"\x1b[<0;38;4M\x1b[<32;50;4M\x1b[<0;50;4m")
        terminal.wait_until(lambda screen: len(screen.splitlines()) > 3 and
                            [i for i, char in enumerate(screen.splitlines()[3]) if char == "│"] == [0, 49, 119],
                            "mouse divider resize", start)
        # Copy rendered pane text through real SGR input and OSC 52 output.
        token = "search-needle-b.go"
        rows = terminal.screen.text().splitlines()
        row = next(i for i, line in enumerate(rows) if token in line)
        col = rows[row].index(token)
        start = len(terminal.output)
        gesture = (f"\x1b[<0;{col+1};{row+1}M"
                   f"\x1b[<32;{col+len(token)};{row+1}M"
                   f"\x1b[<0;{col+len(token)};{row+1}m")
        os.write(terminal.master, gesture.encode())
        terminal.wait_for("copied to clipboard", start)
        payload = base64.b64encode(token.encode())
        terminal.wait_until(lambda screen: b"\x1b]52;c;" + payload in terminal.output[start:],
                            "selected text clipboard write", start)
        terminal.wait_until(lambda screen: "copied to clipboard" not in screen,
                            "clipboard toast dismissal", start)
        terminal.key(b"h", "› [ ] b.go")
        terminal.key(b"\x1b[A", "› [ ] a.go")
        terminal.key(b"P", "Switch pull requests")
        terminal.key(b"\x1b[200~alpha\x1b[201~", "filter: alpha")
        terminal.key(b"\x1b", "0/2 read")
        terminal.key(b"\x1b[B", "› [ ] b.go")
        terminal.key(b"m", "1/2 read")
        # Resizing across the split-pane breakpoint must cause a real repaint.
        start = len(terminal.output)
        terminal.resize(70, 12)
        terminal.wait_until(lambda screen: "› [x] b.go" in screen and
                            all(" | " not in row for row in screen.splitlines()[4:11]),
                            "narrow file pane after resize", start)
        terminal.key(b"?", "Health & help")
        start = len(terminal.output)
        os.write(terminal.master, b"\x1b")
        terminal.wait_until(lambda screen: "1/2 read" in screen and
                            "Health & help" not in screen,
                            "return from help", start)
        terminal.quit(b"\x03")
    print("PASS resume, keyboard marking, resize, help/back, Ctrl+C, restoration")
    print("PASS offline commit picker, legacy source state, and All commits restoration")
    print("PASS drag selection, OSC 52 clipboard payload, and transient copy toast")

    with Terminal(binary, resume, environment) as terminal:
        terminal.wait_for("1/2 read")
        terminal.wait_for("› Overview [1]")
        terminal.key(b"2", "[x] b.go")
        terminal.key(b"1", "› Overview [1]")
        terminal.key(b"v", "› Files [2]")
        terminal.key(b"v", "No guide yet. Press g to generate.")
        terminal.wait_for("› Guide [3]")
        terminal.key(b"4", "Commit fixture 1")
        terminal.wait_for("commit-change-1")
        terminal.key(b"j", "commit-change-2")
        terminal.key(b"l", "commit-change-2")
        terminal.key(b"p", "commit-change-1")
        start = len(terminal.output)
        terminal.resize(70, 12)
        terminal.wait_until(lambda screen: "Commit fixture 1" in screen and
                            "Commit diff" in screen, "narrow commit diff", start)
        terminal.key(b"j", "commit-change-1")
        terminal.key(b"\x1b", "Commits · 1/2")
        terminal.key(b"j", "› Commit fixture 2")
        terminal.key(b"\r", "Commit fixture 2")
        terminal.key(b"j", "commit-change-2")
        terminal.key(b"2", "[x] b.go")
        terminal.wait_for("1/2 read")
        terminal.quit(b"q")
    if gh_called.exists():
        raise AssertionError("browser startup/offline journeys unexpectedly invoked GitHub")
    print("PASS restart/resume preserves marked file without GitHub calls")
    print("PASS offline commit selection, individual diff, narrow focus, and main review restoration")


if __name__ == "__main__":
    main()
