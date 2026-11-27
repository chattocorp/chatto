"""Exercise the packaged listener and a separate focused window on a private Xvfb display."""
import os
import select
import signal
import subprocess
import sys
import time

from Xlib import X, XK, display
from Xlib.ext import xtest


def edge(listener, expected):
    if not select.select([listener.stdout], [], [], 3)[0]:
        raise AssertionError(f"listener did not send {expected}")
    assert listener.stdout.readline().strip() == expected


def key(name, event):
    xtest.fake_input(connection, event, connection.keysym_to_keycode(XK.string_to_keysym(name)))
    connection.sync()


connection = display.Display()
root = connection.screen().root
window = root.create_window(0, 0, 100, 100, 0, X.CopyFromParent, X.InputOutput,
                            X.CopyFromParent, event_mask=X.KeyPressMask | X.KeyReleaseMask)
window.map()
window.set_input_focus(X.RevertToParent, X.CurrentTime)
connection.sync()
env = {**os.environ, "XDG_SESSION_TYPE": "x11"}
env.pop("WAYLAND_DISPLAY", None)
with subprocess.Popen([sys.argv[1], "1", "M", "run.chatto.desktop"], env=env,
                      stdin=subprocess.PIPE, stdout=subprocess.PIPE, bufsize=0) as listener:
    try:
        edge(listener, b"ready")
        key("Control_L", X.KeyPress)
        for hold in range(2):
            key("m", X.KeyPress)
            key("w", X.KeyPress)
            key("w", X.KeyRelease)
            edge(listener, b"down")
            received = []
            deadline = time.monotonic() + 3
            while len(received) < 2:
                if not connection.pending_events() and not select.select(
                    [connection], [], [], max(0, deadline - time.monotonic())
                )[0]:
                    break
                event = connection.next_event()
                if event.type in (X.KeyPress, X.KeyRelease) and event.detail == connection.keysym_to_keycode(XK.XK_w):
                    received.append(event.type)
            assert received == [X.KeyPress, X.KeyRelease], "push-to-talk swallowed foreground input"
            if hold == 0:
                # Hold through Xvfb's autorepeat delay; only one down edge belongs to this hold.
                assert not select.select([listener.stdout], [], [], 1)[0], "unexpected repeat edge"
            key("m", X.KeyRelease)
            edge(listener, b"up")
        # Both edges can reach the server before the helper handles the activation.
        listener.send_signal(signal.SIGSTOP)
        os.waitpid(listener.pid, os.WUNTRACED)
        try:
            key("m", X.KeyPress)
            key("m", X.KeyRelease)
        finally:
            listener.send_signal(signal.SIGCONT)
        edge(listener, b"down")
        edge(listener, b"up")
        key("Control_L", X.KeyRelease)
    finally:
        listener.stdin.close()
        listener.wait(timeout=3)
