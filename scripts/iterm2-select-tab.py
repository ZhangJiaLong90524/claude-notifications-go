#!/usr/bin/env python3
"""Select iTerm2 tab by tmux pane ID for iTerm2 + tmux.

Usage:
    iterm2-select-tab.py --pane %42 --tmux-path /opt/homebrew/bin/tmux
    iterm2-select-tab.py --list

Requires:
    - iterm2 Python module (pip install iterm2)
    - iTerm2 with 'Enable Python API' enabled in Settings > General > Magic
"""
import argparse
import subprocess
import sys

try:
    import iterm2
except ImportError:
    print("iterm2 module not installed. Run: pip install iterm2", file=sys.stderr)
    sys.exit(1)


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pane", help="tmux pane ID, for example %%42")
    parser.add_argument(
        "--tmux-path",
        default="tmux",
        help="absolute path to tmux binary",
    )
    parser.add_argument(
        "--socket",
        default="",
        help="optional tmux socket path from the TMUX environment",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="list iTerm2 tabs with tmuxWindowPane and tty variables",
    )
    args = parser.parse_args()
    if not args.list and not args.pane:
        parser.error("--pane is required unless --list is used")
    return args


def normalized_pane(target_pane):
    target = str(target_pane).strip()
    if not target:
        raise ValueError("tmux pane target is empty")
    if target.startswith("%"):
        return target
    return f"%{target}"


def run_tmux(tmux_path, socket_path, tmux_args):
    cmd = [tmux_path]
    if socket_path:
        cmd.extend(["-S", socket_path])
    cmd.extend(tmux_args)

    result = subprocess.run(
        cmd,
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        stderr = (result.stderr or "").strip()
        raise RuntimeError(stderr or f"tmux command failed: {' '.join(tmux_args)}")
    return (result.stdout or "").strip()


def get_session_client_ttys(tmux_path, socket_path, pane_target):
    session_name = run_tmux(
        tmux_path,
        socket_path,
        ["display-message", "-t", pane_target, "-p", "#{session_name}"],
    )
    if not session_name:
        raise RuntimeError(f"tmux session lookup returned empty for {pane_target}")

    client_lines = run_tmux(
        tmux_path,
        socket_path,
        ["list-clients", "-F", "#{client_session}\t#{client_tty}"],
    )

    client_ttys = []
    for line in client_lines.splitlines():
        parts = line.split("\t", 1)
        if len(parts) != 2:
            continue
        client_session, client_tty = parts
        if client_session == session_name and client_tty:
            client_ttys.append(client_tty)

    if not client_ttys:
        raise RuntimeError(f"no tmux client tty found for session {session_name}")
    return client_ttys


def focus_tmux_client(tmux_path, socket_path, pane_target, client_tty):
    run_tmux(
        tmux_path,
        socket_path,
        ["switch-client", "-c", client_tty, "-t", pane_target],
    )
    run_tmux(
        tmux_path,
        socket_path,
        ["select-window", "-t", pane_target],
    )
    run_tmux(
        tmux_path,
        socket_path,
        ["select-pane", "-t", pane_target],
    )


async def select_tab(connection, target_pane, tmux_path, socket_path):
    """Find and select the iTerm2 tab for the target pane.

    Strategy:
    1. tmuxWindowPane match for tmux -CC mode
    2. tty match for plain tmux, then targeted switch-client/select-window/select-pane
    """
    pane_target = normalized_pane(target_pane)
    pane_number = pane_target[1:] if pane_target.startswith("%") else pane_target
    app = await iterm2.async_get_app(connection)
    tabs = []
    for window in app.windows:
        for tab in window.tabs:
            tab_ttys = set()
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                if wp is not None and str(wp) == pane_number:
                    await window.async_activate()
                    await tab.async_select()
                    return True
                tty = await session.async_get_variable("tty")
                if tty:
                    tab_ttys.add(str(tty))
            tabs.append((window, tab, tab_ttys))

    client_ttys = get_session_client_ttys(tmux_path, socket_path, pane_target)
    for window, tab, tab_ttys in tabs:
        for client_tty in client_ttys:
            if client_tty in tab_ttys:
                await window.async_activate()
                await tab.async_select()
                focus_tmux_client(tmux_path, socket_path, pane_target, client_tty)
                return True
    return False


async def list_tabs(connection):
    """List all iTerm2 tabs with their tmuxWindowPane and tty mappings."""
    app = await iterm2.async_get_app(connection)
    for window in app.windows:
        print(f"Window: {window.window_id}")
        for i, tab in enumerate(window.tabs):
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                tty = await session.async_get_variable("tty")
                print(f"  Tab {i}: tmuxWindowPane={wp} tty={tty}")


async def main(connection):
    args = parse_args()

    if args.list:
        await list_tabs(connection)
        return

    if not await select_tab(connection, args.pane, args.tmux_path, args.socket):
        print(f"No tab found for tmux pane {args.pane}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    try:
        iterm2.run_until_complete(main)
    except Exception as e:
        print(f"iTerm2 API error: {e}", file=sys.stderr)
        print(
            "Ensure 'Enable Python API' is checked in "
            "iTerm2 > Settings > General > Magic",
            file=sys.stderr,
        )
        sys.exit(1)
