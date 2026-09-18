#!/usr/bin/env python3
"""Synthetic pr-review binary used only by internal/verify journey tests."""
import os
import sys
import termios
import tty


SESSION = "0123456789abcdef0123456789abcdef"


def option(name):
    return sys.argv[sys.argv.index(name) + 1]


def main():
    command = sys.argv[1]
    store = option("--store")
    if command == "open":
        print("PR #42 | head aaaaaaaaaaaa | inventory complete")
        print("Session: " + SESSION + " | plan: file-v1")
        return
    if command != "resume" or "--offline" not in sys.argv:
        raise SystemExit(2)
    marked = os.path.exists(os.path.join(store, "marked"))
    tty.setraw(sys.stdin.fileno())
    count = "1/2" if marked else "0/2"
    print("freshness: unchecked | " + count + " read (local)")
    print("unit 1/2")
    print("[x] fake.go" if marked else "[ ] fake.go")
    while True:
        key = os.read(sys.stdin.fileno(), 1)
        if key in (b"q", b"\x03"):
            return
        if key == b"\x1b":
            os.read(sys.stdin.fileno(), 2)
            print("unit 2/2")
        if key == b"m":
            open(os.path.join(store, "marked"), "w").close()
            print("freshness: unchecked | 1/2 read (local)")
            print("[x] fake.go")


if __name__ == "__main__":
    main()
