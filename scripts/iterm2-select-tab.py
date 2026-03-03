#!/usr/bin/env python3
"""Select iTerm2 tab by tmux pane ID (for tmux -CC mode).

Usage:
    iterm2-select-tab.py <pane_number>    Select tab for pane
    iterm2-select-tab.py --list           List all tabs with pane mappings

Requires:
    - iterm2 Python module (pip install iterm2)
    - iTerm2 with 'Enable Python API' enabled in Settings > General > Magic
"""
import sys

try:
    import iterm2
except ImportError:
    print("iterm2 module not installed. Run: pip install iterm2", file=sys.stderr)
    sys.exit(1)


async def select_tab(connection, target_pane):
    """Find and select the iTerm2 tab whose tmuxWindowPane matches target_pane."""
    app = await iterm2.async_get_app(connection)
    for window in app.windows:
        for tab in window.tabs:
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                if wp is not None and str(wp) == target_pane:
                    await window.async_activate()
                    await tab.async_select()
                    return True
    return False


async def list_tabs(connection):
    """List all iTerm2 tabs with their tmuxWindowPane mappings."""
    app = await iterm2.async_get_app(connection)
    for window in app.windows:
        print(f"Window: {window.window_id}")
        for i, tab in enumerate(window.tabs):
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                print(f"  Tab {i}: tmuxWindowPane={wp}")


async def main(connection):
    if len(sys.argv) == 2 and sys.argv[1] == "--list":
        await list_tabs(connection)
        return

    target = sys.argv[1]
    if not await select_tab(connection, target):
        print(f"No tab found for tmux pane {target}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <pane_number> | --list", file=sys.stderr)
        sys.exit(1)
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
