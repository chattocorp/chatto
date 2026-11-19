# FDR-059: Client Experimental Features

**Status:** Active
**Last reviewed:** 2026-11-19

## Behavior

In App Preferences, open Experiments and select a feature. Each feature starts disabled. A change takes effect immediately on every connected server in this app. Tabs that share the same browser local storage also apply the change. The browser remembers the choice after a reload.

With **New message header** enabled, the header shows the author’s display name followed by their @username. A muted midpoint separates the names from the timestamp. The names and timestamp use the same text size and stay on one line. The username truncates first. If fewer than three username characters fit after the @ sign, the header hides the username. A short username can still appear in full. If space is still limited, the display name also truncates so the midpoint and timestamp remain visible. Consecutive messages with a compact layout keep their compact timestamp.

## Rationale

A local opt-in lets a user try a presentation change across their servers and return to the usual layout at any time. Each browser can have its own choices.
