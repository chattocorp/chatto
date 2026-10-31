# FDR-001: Agent interactions and steering

**Status:** Experimental
**Last reviewed:** 2026-10-21

## Overview

Runling supports agent tasks and explicit sessions for custom conversation
loops and structured outcomes. `agent()` defines a task. Each invocation owns
one session, gives parent messages to it, and emits its completed replies.

## Behavior

- Completed-text callbacks identify messages that contain tool calls. Hosts can
  deliver interim text during work and deliver the returned answer after cleanup.

- Agent task definitions are reusable and do not start a session at definition
  time. Each invocation owns setup, execution, cancellation, and disposal.
- Agent tasks accept string prompts by default. A typed input schema requires
  a prompt function. Optional async setup receives the validated input and the
  invocation context.
- Agent tasks return final text and emit completed assistant replies. They use
  steering during an active turn. Read messages that were not delivered become
  later turns in the same session. Open input does not keep an idle task alive.
- `runling run` records assistant replies and emitted messages in its journal.
  Terminal display follows [FDR-011](FDR-011-cli-output.md).
- `createAgentSession()` exposes explicit session ownership for custom loops,
  report outcomes, and forks. `runAgent()` runs one explicit interaction.
- Model lookup uses the local catalog first. If a model is missing, refresh only
  its provider catalog with a five-second deadline, then try lookup again. This
  lets new models work without an SDK update and avoids network discovery for
  known models. `PI_OFFLINE` disables network discovery.
- Text mode lets the agent finish its turn naturally. The final text of the
  agent is the summary of the result. Applications can also see each completed
  assistant text message during the turn. These messages do not include private
  reasoning or tool results.
- Report mode requires a structured outcome and permits one format-repair attempt.
  Provider failure and a missing report are host failures, distinct from a model
  reporting that it is blocked.
- An empty text response gives a failed outcome by default. A final tool batch
  whose results all succeed and return `terminate: true` completes text mode
  without a reply. Nested tool results cannot terminate the parent batch.
  Earlier batches and interactions do not grant completion to a later empty turn.
  Text agents can set `allowEmptyText: true` to permit intentional silence even
  without a terminating tool. Provider errors still fail in both cases.
- With live logging enabled, reasoning summaries and, outside `runling run`,
  agent replies appear as Markdown
  blocks below their agent ID. Text wraps to the available width after task
  indentation. Journals and pipes keep the original Markdown. `NO_COLOR`
  disables terminal colors, including in the final workflow output.
- Steering enters before a subsequent model call. A positive delivery result
  does not mean the model followed the instruction. The caller keeps the
  messages that the agent did not get, for example for the next turn.
- Delivery of a steering message cancels an earlier outcome report of the same
  turn. The agent must report again.
- An agent cannot run two turns at the same time.
- Session creation binds native extensions before returning. Asynchronous
  disposal waits for extension shutdown and then releases Pi resources. This
  lets hosts close native MCP transports before they release shared resources.
- Hosts can select extension tools without built-in tools, and can require
  sequential tool execution when preparation hooks use a shared lock.
- Hosts can omit model content and tool arguments from logs and progress
  events while retaining usage and tool activity. Private transcripts and
  explicit task data remain the host's responsibility.
- A session file keeps the conversation of an agent. A new agent with the same
  file continues the conversation. A fork copies the conversation into a new
  agent in memory.
- Abort signals and disposal cancel pending work. Cancellation cannot undo a
  tool's external effects or stop uncooperative code.
- Cancelling an agent task starts session disposal immediately so native tool
  transports can close. The task joins that disposal before cleanup finishes;
  it does not wait for a blocked tool before starting transport shutdown.
- `awaitWithSignal` lets tasks stop waiting for an external operation that has
  no cancellation API. It checks the signal before starting work and removes
  its listener when the wait ends. It cannot stop the external operation; the
  application owns cleanup and the decision to retry uncertain writes.
- An application can give a task to an agent as a tool. The model gets the
  final result of the task as text and its streamed output as partial results. Cancellation
  of the tool call cancels the task.
- Background child tasks let a parent continue to answer messages while work
  runs. These are ordinary invocations. The parent keeps the handles or promises
  it needs, consumes progress, and decides how to deliver results. It owns waiting
  and disposal; Runling does not provide a separate job tracker or child registry.
  Child work is not retried or restored after restart.
- `createNotifier` wakes a process-local loop that owns its queue or state. It
  retains at most one notification, permits one pending waiter, and supports
  cancellation and an optional timeout. It cleans up the wait's timer and listener.
  This avoids repeated promise and timer plumbing in application loops; batching,
  data, and recovery remain the caller's responsibility. See [task channels](../task-channels.md#wake-a-processing-loop).
- Classifiers run as ordinary tasks. `createClassifier` runs one tool-free agent
  invocation and lets callers omit model content from logs with `logContent: false`.
  It performs one turn with no local instructions or saved history. `createChoiceClassifier`
  sends one typed choice question to OpenRouter's Decisions API without an agent
  session. The caller supplies the input schema, policy, model, and named choices;
  the choice result includes provider probabilities and confidence. Both have a
  30-second request deadline. Malformed output, model failures, refusals, and
  cancellation fail the task; there is no retry or automatic fallback. The choice
  adapter records reported token usage and cost in the task context. Classification
  logs show the model, choice, probabilities, confidence, duration, and reported
  usage and cost under the task's run context, without input text or credentials.
  Classification policy, thresholds, and any authorization based on it belong to
  the application.
- `createDescription` defines a tool-free task that describes caller-supplied
  text. The caller selects the model, instructions, and reasoning level (off by
  default). Each call has a fresh session, no local instructions or saved history,
  no content logs, and a 30-second deadline. Normal task observation records usage
  and cost. It returns trimmed text, which can be empty when the instructions
  permit omission. Failures and cancellation propagate without retries. The
  caller owns any title updates, persistence, and fallback policy.
- Shared agent sessions can be resumed across workflows with an explicit ID.
  Only one lease in the process can use a file at a time. Scopes have separate
  files. Opening a shared session renews its activity and removes other inactive
  session files older than 30 days in the same scope. Leased files are preserved.
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

Agent resource options can select explicit skill paths instead of Pi's default
discovery. Pi owns skill parsing and catalog instructions. The host selects tools
and enforces any file-access restrictions; skill discovery grants no access.

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
**Tradeoff:** Applications own the conversation loop, pending-child bookkeeping,
notification routing, and approval policy. Framework helpers provide isolated
classification and session storage without interpreting application policy.

## Related

- **ADRs:** [ADR-002](../adr/ADR-002-agent-ownership.md),
  [ADR-009](../adr/ADR-009-task-invocations.md)
- **FDRs:** [FDR-010](FDR-010-host-messages.md), [FDR-011](FDR-011-cli-output.md)
- [Agent API guide](../agents.md) and [steering guide](../agent-steering.md)

## Open Questions

- Should the Log view offer a filter for specific log levels?
