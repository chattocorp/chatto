# FDR-001: Agent interactions and steering

**Status:** Experimental
**Last reviewed:** 2026-09-23

## Overview

Runling supports conversational agents and specialist agents with structured
outcomes. Connections deliver live input and ordered assistant output.

## Behavior

- Text mode delivers completed assistant messages through text callbacks. The
  returned summary is not a second message to send when the callback sent it.
- Report mode requires a structured outcome and permits one format-repair attempt.
  Provider failure and a missing report are host failures, distinct from a model
  reporting that it is blocked.
- Text callbacks exclude private reasoning and tool results. A turn waits for
  queued asynchronous text delivery before returning.
- Applications can restrict text delivery to normally stopped assistant messages,
  excluding tool-call preambles and failed responses. Intermediate text remains
  available in agent logs.
- Steering enters before a subsequent model call. A positive consumption receipt
  does not mean the model followed the instruction. Rejected or unconsumed input
  is the caller's responsibility.
- Connections reject overlapping turns. The conversation coordinator retains
  unconsumed user input and handles task notifications separately.
- Abort signals and disposal cancel pending work. Cancellation cannot undo a
  tool's external effects or stop uncooperative code.
- The console opens each run on its recorded Log view. It shows log lines and
  task milestones in journal order, with time,
  level, indentation, and ANSI colors. New lines keep the view at the bottom
  until the reader scrolls up. The reader can resume following and load older
  lines. Long lines show four rows until the reader expands them. Timeline is
  the other run tab. Input, result, and usage are in Run details. A static icon
  marks a run that waits for input with no other active work. A running run
  with active work shows its quiet time after two minutes without a recorded event.

## Design Decisions

### 1. Choose output behavior explicitly

**Decision:** Use text mode for natural replies and report mode for structured tasks.
**Why:** User-facing replies and machine-readable outcomes have different needs.
**Tradeoff:** The caller must choose the mode and avoid sending the same output twice.

### 2. Track consumption separately

**Decision:** Distinguish message acceptance from agent consumption.
**Why:** Messages can arrive while the agent is busy or between turns.
**Tradeoff:** A connection alone does not guarantee eventual delivery; the owner
must retain or reroute missed input.

## Related

- [ADR-002: Agent ownership](../adr/ADR-002-agent-ownership.md)
- [Agent API guide](../agents.md) and [steering guide](../agent-steering.md)

## Open Questions

- Should the Log view offer a filter for specific log levels?
