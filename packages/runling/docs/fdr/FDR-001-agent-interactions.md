# FDR-001: Agent interactions and steering

**Status:** Experimental
**Last reviewed:** 2026-10-14

## Overview

Runling supports conversational agents and specialist agents with structured
outcomes. An agent is the implementation of a task. The task gives messages
from its parent to the agent and emits the replies of the agent.

## Behavior

- Text mode lets the agent finish its turn naturally. The final text of the
  agent is the summary of the result. Applications can also see each completed
  assistant text message during the turn. These messages do not include private
  reasoning or tool results.
- Report mode requires a structured outcome and permits one format-repair attempt.
  Provider failure and a missing report are host failures, distinct from a model
  reporting that it is blocked.
- An empty text response or a provider error gives a failed outcome.
- Steering enters before a subsequent model call. A positive delivery result
  does not mean the model followed the instruction. The caller keeps the
  messages that the agent did not get, for example for the next turn.
- Delivery of a steering message cancels an earlier outcome report of the same
  turn. The agent must report again.
- An agent cannot run two turns at the same time.
- A session file keeps the conversation of an agent. A new agent with the same
  file continues the conversation. A fork copies the conversation into a new
  agent in memory.
- Abort signals and disposal cancel pending work. Cancellation cannot undo a
  tool's external effects or stop uncooperative code.
- An application can give a task to an agent as a tool. The model gets the
  output of the task as text and its updates as partial results. Cancellation
  of the tool call cancels the task.
- The console opens each run on its recorded Log view. It shows log lines and
  task milestones in journal order, with time, level, indentation, branch
  guides, and ANSI colors. Task milestones use their task depth so the guides
  show their position in the run. New lines keep the view at the bottom
  until the reader scrolls up. The reader can resume following and load older
  lines. Long lines show four rows until the reader expands them. Timeline is
  the other run tab. On wide screens, a details pane sits beside the log.
  Task references and agent names in the log select the activity shown there.
  With no activity selected, the pane shows the run input, result, and usage.
  Smaller screens use Run details and a task dialog. A static icon marks a run
  that waits for a message with no other active work. A running run
  with active work shows its quiet time after two minutes without a recorded event.

## Design Decisions

### 1. Choose output behavior explicitly

**Decision:** Use text mode for natural replies and report mode for structured tasks.
**Why:** User-facing replies and machine-readable outcomes have different needs.
**Tradeoff:** The caller must choose the mode.

### 2. Track delivery separately

**Decision:** Distinguish message acceptance from delivery to the agent.
**Why:** Messages can arrive while the agent is busy or between turns.
**Tradeoff:** Steering alone does not guarantee eventual delivery. The task that
owns the agent must keep or reroute missed messages.

### 3. An agent is a task, not a separate kind of execution

**Decision:** Agents use the same invocations and messages as other tasks.
Runling has no agent supervision layer.
**Why:** One invocation model is easier to learn and to change. Supervision
policy belongs to the application ([ADR-009](../adr/ADR-009-task-invocations.md)).
**Tradeoff:** Applications that need background agents, notifications, or
approvals implement them with tasks and messages.

## Related

- **ADRs:** [ADR-002](../adr/ADR-002-agent-ownership.md),
  [ADR-009](../adr/ADR-009-task-invocations.md)
- **FDRs:** [FDR-010](FDR-010-host-messages.md)
- [Agent API guide](../agents.md) and [steering guide](../agent-steering.md)

## Open Questions

- Should the Log view offer a filter for specific log levels?
