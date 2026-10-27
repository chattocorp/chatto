# FDR-011: CLI run details and final output

**Status:** Experimental
**Last reviewed:** 2026-10-15

## Overview

The `runling run` command shows enough activity to follow a run, then shows
the root task's final result once. Operators can distinguish work in progress
from the answer and use the journal to inspect intermediate replies.

## Behavior

- Workspace CLI commands load TypeScript source without a prior Runling build.
  Workspace serving uses Vite; npm installs use the compiled console. Both paths
  share the host lifecycle. See [ADR-013](../adr/ADR-013-source-workspace-exports.md).

- Runling owns application logging through `log`, including structured fields and
  warning levels. Workflows, sources, and server diagnostics share one terminal
  renderer. Source logs carry the source name; workflow logs carry run and task
  context and remain in the run journal. Separate destinations do not require
  application formatters. Applications must exclude private data from log fields.
- The server prints the root task's start once. Successful HTTP responses are
  debug diagnostics. `RUNLING_LOG_LEVEL=debug` enables debug logs for the server,
  sources, and workflows. Verbose executions can also enable debug for their own work.
- Normal output shows the run reference at startup, task and step progress,
  and agent activity, including tool use. Agent reasoning summaries can appear
  as part of this activity. `--verbose` also shows debug logs.
- At completion, the command shows accumulated token usage when tokens were
  used, the duration, and the journal path when the journal was saved.
- After these details, a successful run prints its non-null root result once.
  A blank line separates the result from the run details. The result has no
  log marker, agent prefix, or task indentation. Its own Markdown structure
  can contain indentation.
- Intermediate assistant replies, child task results, and emitted updates
  do not print as answers. The journal retains assistant replies and emitted
  updates. An emitted reply that is also the final result still appears only
  once in terminal output.
- Steps inside agent tools use the same agent ID prefix and color as the
  surrounding agent activity. The agent ID identifies the agent; the run
  reference identifies the whole run. Tool authors supply the step label.
- On a terminal, string results render as Markdown and wrap to the available
  width. Redirected string results retain their Markdown. Other results print
  as JSON. `NO_COLOR` disables terminal colors.
- With `--json`, stdout contains one execution document with the result,
  error, duration, usage, and success status. Run details go to stderr. There
  is no separate answer block or blank separator on stdout.
- Failures produce diagnostics on stderr and a nonzero exit status. A failed
  run does not print a successful answer. JSON mode also reports failures
  that prevent a workflow from starting.
- Interactive input remains available, but the command does not print the
  "Type a line and press Enter" hint. Message delivery follows
  [FDR-010](FDR-010-host-messages.md).

## Design Decisions

### 1. Keep run details visible

**Decision:** Show operational activity even when intermediate answers are hidden.
**Why:** Operators need to know whether the run is working, which tools it uses,
and where to inspect it. Removing duplicate answers must not silence the run.
**Tradeoff:** Normal terminal output contains both activity and the final result.
Use JSON mode when another program needs a structured result.

### 2. The root task owns the final answer

**Decision:** Print only the root result as the answer, after all run details.
**Why:** A root task can combine or replace child results. Printing agent replies,
updates, and the root result repeats content and can present intermediate work
as the answer. The result remains the task's output, as defined by
[ADR-009](../adr/ADR-009-task-invocations.md).
**Tradeoff:** Intermediate replies are available through the journal instead of
the live terminal. Interactive workflows that need to display questions as
updates need a host that displays those updates, such as the web console.

### 3. Separate activity from the answer visually

**Decision:** Keep activity labels on log lines, preserve the agent prefix on
tool steps, and place a blank line before the unprefixed final answer.
**Why:** Readers can identify who is working and where the answer starts.
**Tradeoff:** Tool authors must choose useful activity labels. A label such as
`Executing: ...` explains an action; Runling does not infer that action from
arbitrary tool arguments.

### 4. Keep the journal complete

**Decision:** Terminal display choices do not remove replies or updates from
the journal.
**Why:** Operators need the intermediate evidence to diagnose a run even when
the terminal shows only the final answer.
**Tradeoff:** The journal contains more detail than the terminal.

## Related

- **ADRs:** [ADR-009](../adr/ADR-009-task-invocations.md)
- **FDRs:** [FDR-001](FDR-001-agent-interactions.md),
  [FDR-010](FDR-010-host-messages.md)
- Guides: [workflow messages and output](../workflow-messages.md),
  [agents](../agents.md), [server logs](../server-logs.md)
