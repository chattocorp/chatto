# FDR-059: Client Experimental Features

**Status:** Active
**Last reviewed:** 2026-11-19

## Behavior

In App Preferences, open Appearance and select a feature under Experimental Features. Each feature starts disabled. A change takes effect immediately on every connected server in this app. Tabs that share the same browser local storage also apply the change. The browser remembers the choice after a reload.

With **New message header** enabled, the author and badges appear on the first line. The time and other message metadata appear below them. Consecutive messages with a compact layout keep their compact timestamp.

## Rationale

A local opt-in lets a user try a presentation change across their servers and return to the usual layout at any time. Each browser can have its own choices.
