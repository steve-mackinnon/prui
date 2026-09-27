"""Fixed PTY journey for pr-review verify. Python standard library only."""
import base64
import codecs
import errno
import fcntl
import json
import os
import pty
import re
import selectors
import signal
import struct
import subprocess
import sys
import termios
import time


MAX_OUTPUT = 2 * 1024 * 1024


class Screen:
    def __init__(self, width=120, height=24):
        self.width, self.height = width, height
        self.row = self.col = 0
        self.pending = ""
        self.decoder = codecs.getincrementaldecoder("utf-8")()
        self.cells = [[" "] * width for _ in range(height)]

    def feed(self, data):
        self.pending += self.decoder.decode(data)
        while self.pending:
            if self.pending.startswith("\x1b["):
                match = re.match(r"\x1b\[([0-?]*[ -/]*)([@-~])", self.pending)
                if not match:
                    return
                self.csi(*match.groups())
                self.pending = self.pending[match.end():]
                continue
            if self.pending.startswith("\x1b]"):
                match = re.match(r"\x1b\].*?(?:\x07|\x1b\\\\)", self.pending, re.S)
                if not match:
                    return
                self.pending = self.pending[match.end():]
                continue
            if self.pending.startswith("\x1b"):
                if len(self.pending) < 2:
                    return
                if self.pending[1] == "M":
                    if self.row == 0:
                        self.cells.insert(0, [" "] * self.width)
                        self.cells.pop()
                    else:
                        self.row -= 1
                    self.pending = self.pending[2:]
                    continue
                raise RuntimeError("unsupported terminal escape")
            char, self.pending = self.pending[0], self.pending[1:]
            if char == "\r": self.col = 0
            elif char == "\n": self.linefeed()
            elif char == "\b": self.col = max(0, self.col - 1)
            elif char >= " ":
                if self.col >= self.width:
                    self.col = 0
                    self.linefeed()
                self.cells[self.row][self.col] = char
                self.col += 1

    def csi(self, params, final):
        if any(c not in "0123456789;" for c in params): return
        values = [int(v or 0) for v in params.split(";")]
        n = values[0] or 1
        if final in "Hf": self.row, self.col = n - 1, (values[1] or 1) - 1 if len(values) > 1 else 0
        elif final == "A": self.row -= n
        elif final == "B": self.row += n
        elif final == "C": self.col += n
        elif final == "D": self.col -= n
        elif final in "G`": self.col = n - 1
        elif final == "J":
            mode = values[0]
            if mode in (2, 3): self.cells = [[" "] * self.width for _ in range(self.height)]
            elif mode == 0:
                self.cells[self.row][self.col:] = [" "] * (self.width - self.col)
                for row in range(self.row + 1, self.height): self.cells[row] = [" "] * self.width
            elif mode == 1:
                for row in range(self.row): self.cells[row] = [" "] * self.width
                self.cells[self.row][:self.col + 1] = [" "] * (self.col + 1)
        elif final == "K":
            start, end = (0, self.width) if values[0] == 2 else ((0, self.col + 1) if values[0] == 1 else (self.col, self.width))
            self.cells[self.row][start:end] = [" "] * (end - start)
        elif final == "X":
            end = min(self.width, self.col + n)
            self.cells[self.row][self.col:end] = [" "] * (end - self.col)
        self.row, self.col = max(0, min(self.row, self.height - 1)), max(0, min(self.col, self.width - 1))

    def linefeed(self):
        self.row += 1
        if self.row >= self.height:
            self.cells.pop(0); self.cells.append([" "] * self.width); self.row = self.height - 1

    def text(self):
        return "\n".join("".join(row).rstrip() for row in self.cells)


class Terminal:
    def __init__(self, binary, args, timeout):
        self.master, self.slave = pty.openpty()
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 120, 0, 0))
        self.output, self.screen, self.timeout = bytearray(), Screen(), timeout
        self.selector = selectors.DefaultSelector(); self.selector.register(self.master, selectors.EVENT_READ)
        env = os.environ.copy(); env["TERM"] = "xterm-256color"
        self.process = subprocess.Popen([binary, *args], stdin=self.slave, stdout=self.slave, stderr=self.slave, start_new_session=True, env=env)

    def close(self):
        if self.process.poll() is None:
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait(timeout=self.timeout)
        self.selector.close(); os.close(self.master); os.close(self.slave)

    def read(self, timeout):
        if not self.selector.select(timeout): return False
        try: data = os.read(self.master, 65536)
        except OSError as error:
            if error.errno == errno.EIO: return False
            raise
        if not data: return False
        self.output.extend(data)
        if len(self.output) > MAX_OUTPUT: raise RuntimeError("terminal output exceeded limit")
        self.screen.feed(data)
        return True

    def wait(self, pattern):
        expression, deadline = re.compile(pattern), time.monotonic() + self.timeout
        while True:
            found = expression.search(self.screen.text())
            if found: return found
            if self.process.poll() is not None: raise RuntimeError("terminal exited before expected screen")
            if time.monotonic() >= deadline: raise RuntimeError("terminal timed out waiting for expected screen")
            self.read(0.05)

    def key(self, value): os.write(self.master, value)

    def quit(self):
        self.key(b"q")
        deadline = time.monotonic() + self.timeout
        while self.process.poll() is None:
            if time.monotonic() >= deadline: raise RuntimeError("terminal did not quit")
            self.read(0.05)
        if self.process.returncode: raise RuntimeError("terminal quit with failure")


def one_run(binary, session, timeout):
    started = time.monotonic_ns()
    first = Terminal(binary, ["resume", session, "--offline"], timeout)
    try:
        process_start_ns = time.monotonic_ns() - started
        match = first.wait(r"(\d+)/(\d+) read \(local\)")
        first_frame_ns = time.monotonic_ns() - started
        before, total = int(match.group(1)), int(match.group(2))
        initial = first.screen.text()
        if total > 1: first.key(b"\x1b[B")
        first.key(b"m")
        first.wait(r"%d/%d read \(local\)" % (before + 1, total))
        marked = first.screen.text()
        first.quit()
        transcript = bytes(first.output)
    finally: first.close()
    resumed = Terminal(binary, ["resume", session, "--offline"], timeout)
    try:
        resumed.wait(r"%d/%d read \(local\)" % (before + 1, total))
        resumed.wait(r"\[x\]")
        screen = resumed.screen.text()
        resumed.quit()
        transcript += bytes(resumed.output)
    finally: resumed.close()
    return {"transcript_base64": base64.b64encode(transcript).decode(), "screens": {"initial": initial, "marked": marked, "resumed": screen}, "process_start_ns": process_start_ns, "first_review_frame_ns": first_frame_ns}


def main():
    binary, session, timeout = sys.argv[1:]
    print(json.dumps(one_run(binary, session, float(timeout)), separators=(",", ":")))


if __name__ == "__main__": main()
