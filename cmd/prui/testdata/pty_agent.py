"""Separate local-provider PTY journey for the private Agent Q&A feature."""

import os
import pathlib
import sys
import time

from pty_smoke import Terminal


SOURCE = "func target() bool { return true }"


def mouse_click(terminal, token):
    rows = terminal.screen.text().splitlines()
    matches = [(row, line.index(token)) for row, line in enumerate(rows)
               if token in line and ("+" in line or token == "pkg/work_test.go:1-1")]
    if not matches:
        raise AssertionError("target row was not visible")
    row, col = matches[0]
    event = f"\x1b[<0;{col+1};{row+1}M\x1b[<0;{col+1};{row+1}m".encode()
    os.write(terminal.master, event)
    terminal.wait_until(lambda screen: "RIGHT:1" in screen or "RIGHT 1" in screen,
                        "canonical source selection")
    return row


def open_agent(terminal):
    mouse_click(terminal, SOURCE)
    terminal.key(b"\r", "[Comment]")
    terminal.key(b"\t", "[Ask agent]")


def send_question(terminal, question, expected):
    open_agent(terminal)
    terminal.key(b"\x1b[200~" + question.encode() + b"\x1b[201~", question)
    start = len(terminal.output)
    os.write(terminal.master, b"\r")
    terminal.wait_for(expected, start)


def inspect_test_citation(terminal):
    # Files j/k crosses each displayed row, including Agent cards. Drive the
    # cursor from the selected source line to the exact reference row.
    source_row = mouse_click(terminal, SOURCE)
    rows = terminal.screen.text().splitlines()
    citations = [i for i, line in enumerate(rows) if "pkg/work_test.go:1-1" in line]
    if not citations or citations[0] <= source_row:
        raise AssertionError("captured test reference was not visible below its anchor")
    for _ in range(citations[0] - source_row):
        os.write(terminal.master, b"j")
        terminal.read(0.02)
    start = len(terminal.output)
    os.write(terminal.master, b"\r")
    terminal.wait_for("Sent evidence", start)
    terminal.wait_for("func TestTarget() { assert(target()) }")
    terminal.key(b"\x1b", "Agent Q&A")


def main():
    binary, home, session_id = sys.argv[1:]
    root = pathlib.Path(home)
    bin_dir = root / "fake-bin"
    bin_dir.mkdir()
    # All GitHub calls are locally refused. The log proves the journey never
    # requested a write, even if an online freshness check attempts a read.
    gh = bin_dir / "gh"
    gh.write_text(
        '#!/bin/sh\n'
        'printf "%s\\n" "$*" >> "$HOME/gh-agent-calls"\n'
        'case "$*" in\n'
        '  "api --hostname github.com graphql --input -")\n'
        '    while IFS= read -r line || [ -n "$line" ]; do\n'
        '      case "$line" in *mutation*) : > "$HOME/gh-write-attempt" ;; esac\n'
        '    done\n'
        '    ;;\n'
        'esac\n'
        'exit 97\n'
    )
    gh.chmod(0o700)
    environment = {
        "TERM": "xterm-256color", "PATH": str(bin_dir), "HOME": str(root),
        "XDG_CONFIG_HOME": str(root), "XDG_DATA_HOME": str(root),
        "GH_CONFIG_DIR": str(root), "LC_ALL": "C", "NO_COLOR": "1",
    }
    stage = "startup"
    try:
        with Terminal(binary, ["resume", session_id], environment) as terminal:
            terminal.resize(140, 50)
            terminal.wait_for("› Overview [1]")
            terminal.key(b"2", "› Files [2]")
            terminal.wait_for(SOURCE)
            stage = "hunk question"
            send_question(terminal, "HUNK question about target", "Hunk answer from selected code.")
            stage = "captured test question"
            send_question(terminal, "TEST question about target", "Captured test evidence supports target.")
            stage = "inspect exact evidence"
            inspect_test_citation(terminal)
            stage = "missing caller question"
            send_question(terminal, "CALLER question about target", "Caller evidence was not supplied.")
            stage = "follow-up"
            send_question(terminal, "FOLLOW question about target", "Follow-up uses the anchored hunk.")
            stage = "Guide anchor continuation"
            terminal.key(b"3", "› Guide [3]")
            terminal.wait_for(SOURCE)
            send_question(terminal, "GUIDE question about target", "Guide reopens the same anchored thread.")
            terminal.key(b"2", "› Files [2]")
            stage = "cancel slow request"
            open_agent(terminal)
            terminal.key(b"\x1b[200~SLOW question about target\x1b[201~", "SLOW question")
            start = len(terminal.output)
            os.write(terminal.master, b"\r")
            terminal.wait_for("Agent is answering", start)
            start = len(terminal.output)
            os.write(terminal.master, b"X")
            terminal.wait_until(lambda screen: "Agent is answering" not in screen and
                                "X: cancel pending" not in screen and "Enter: follow up" in screen,
                                "pending agent state cleared by X", start)
            # The fake provider observes its request context canceled while
            # the terminal is still open, then attempts a late response.
            terminal.wait_until(lambda _: (root / "provider-canceled").exists(),
                                "provider observed request cancellation", start)
            stage = "canceled question retry draft"
            terminal.key(b"\x1b[H", SOURCE)
            open_agent(terminal)
            terminal.wait_for("SLOW question about target")
            start = len(terminal.output)
            os.write(terminal.master, b"\x1b")
            terminal.wait_until(lambda screen: "[Ask agent]" not in screen and
                                "Agent is answering" not in screen,
                                "canceled draft retained after closing editor", start)
            # A canceled provider call cannot add a late answer or retry.
            deadline = time.monotonic() + 0.4
            while time.monotonic() < deadline:
                terminal.read(0.05)
            if "Late slow answer" in terminal.screen.text():
                raise AssertionError("canceled answer appeared")
            terminal.quit(b"\x03")
    except (AssertionError, OSError, TimeoutError) as error:
        # Terminal.fail includes a raw screen/transcript. It can contain source
        # and questions, so the Go test receives only a stage label.
        if os.environ.get("PRUI_AGENT_PTY_DEBUG"):
            raise
        raise SystemExit(f"PTY agent journey failed at {stage}") from None
    gh_log = root / "gh-agent-calls"
    if gh_log.exists():
        calls = gh_log.read_text().splitlines()

        def read_only(call):
            if call in {"auth token --hostname github.com",
                        "api --hostname github.com graphql --input -"}:
                return True
            parts = call.split()
            if parts[:3] != ["api", "--hostname", "github.com"]:
                return False
            # gh api defaults to GET without request fields. An explicit GET
            # is also safe; no other method or flags enter this allowlist.
            if len(parts) == 4:
                return parts[3].startswith("repos/owner/agent/")
            return (len(parts) == 6 and parts[3:5] == ["--method", "GET"]
                    and parts[5].startswith("repos/owner/agent/"))

        unexpected = [call for call in calls if not read_only(call)]
        if unexpected:
            kind = " ".join(unexpected[0].split()[:5])
            raise SystemExit(f"PTY agent journey requested an unexpected GitHub operation ({kind})")
    if (root / "gh-write-attempt").exists():
        raise SystemExit("PTY agent journey requested a GitHub mutation")
    print("PASS local agent PTY hunk, captured test citation, missing caller, follow-up, Guide, cancellation")


if __name__ == "__main__":
    main()
