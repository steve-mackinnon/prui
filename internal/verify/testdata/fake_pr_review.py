#!/usr/bin/env python3
"""Synthetic pr-review binary used only by internal/verify journey tests."""
import os
import json
import sys
import termios
import time
import tty


SESSION = "0123456789abcdef0123456789abcdef"


def option(name):
    return sys.argv[sys.argv.index(name) + 1]


def main():
    command = sys.argv[1]
    store = option("--store")
    if command == "open":
        time.sleep(float(os.environ.get("PR_REVIEW_FAKE_OPEN_DELAY_SECONDS", "0")))
        if os.environ.get("PR_REVIEW_VERIFY_TIMING") == "1":
            print("pr-review-verify-timing:v1 " + json.dumps({"github_metadata_ns": 1000000, "pin_and_inventory_ns": 2000000}), file=sys.stderr)
        print("PR #42 | head aaaaaaaaaaaa | inventory complete")
        print("Session: " + SESSION + " | plan: file-v1")
        return
    if command != "resume" or "--offline" not in sys.argv:
        raise SystemExit(2)
    if os.environ.get("TERM") == "dumb":
        raise SystemExit(3)
    marked = os.path.exists(os.path.join(store, "marked"))
    tty.setraw(sys.stdin.fileno())
    time.sleep(0.02)
    count = "2/2" if os.path.exists(os.path.join(store, "complete")) else ("1/2" if marked else "0/2")
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
            already_marked = os.path.exists(os.path.join(store, "marked"))
            open(os.path.join(store, "marked"), "w").close()
            if already_marked:
                open(os.path.join(store, "complete"), "w").close()
            print("freshness: unchecked | " + ("2/2" if already_marked else "1/2") + " read (local)")
            print("[x] fake.go")


if __name__ == "__main__":
    main()
