# FDR-059: Client Experimental Features

**Status:** Active
**Last reviewed:** 2026-11-21

## Behavior

In App Preferences, open Experiments and select a feature. Each feature starts disabled. A change takes effect immediately on every connected server in this app. Tabs that share the same browser local storage also apply the change. The browser remembers the choice after a reload.

With **New message header** enabled, the header shows the author’s display name followed by their @username. In the timeline, a smaller, muted timestamp follows the author’s names, separated by a middot (·). The names and timestamp stay on one line. The username truncates first. If fewer than three username characters fit after the @ sign, the header hides the username. A short username can still appear in full. If space is still limited, the display name also truncates so the timestamp remains visible. Search results and pinned messages shorten long dates to keep the author visible. Hover over a sidebar result or pin, or over the date in server search, to see the full date and time. Consecutive messages with a compact layout keep their compact timestamp.

With **GLSL shader previews** enabled, message code blocks labelled `glsl` offer a Render tab, and `glsl rendered` shows only the preview. [FDR-032](FDR-032-message-formatting.md) describes the preview. Without the experiment, these blocks are ordinary code. The choice applies only to messages in this app; other people see the blocks according to their own choice.

## Rationale

A local opt-in lets a user try a presentation change across their servers and return to the usual layout at any time. Each browser can have its own choices.
