# FDR-008: Codemode

**Status:** Experimental
**Last reviewed:** 2026-10-03

## Overview

The `codemode` agent option adds Pi's `codemode` tool. The model writes a
JavaScript script that calls the agent's tools, and only the script's output
reaches the model. A script can run several calls in parallel and filter large
results before the model reads them.

## Behavior

- `codemode: true` or `codemode: { mode: 'on' }` adds the `codemode` tool next to
  the agent's other tools. `codemode: { mode: 'only' }` hides the other tools
  from the model, which then reaches them through scripts.
- Scripts run in Pi's QuickJS sandbox. They have no Node APIs, files, network, or
  timers, and reach the outside only through tools.
- A script can call the tools in the agent's `tools` option as
  `tools.<name>(args)`. Pi's built-in tools and extension tools that are not in
  `tools` are not available. `report_outcome` is not available to scripts either,
  so only the model reports an outcome. In `only` mode, the model still sees
  `report_outcome`.
- Scripts cannot use Pi's `models` API, so they cannot start classifier or image
  model requests that cost money.
- Every call from a script passes the same `tool_call` and `tool_result` hooks as
  a call from the model. Trust policies, authorization gates, and other
  extension gates apply to each call. A blocked call rejects in the script with
  the block reason.
- After an `untrusted` tool returns in a script, later calls to blocked tools
  fail, in the same script and in later turns. Calls that the script started
  before that result arrived are not blocked, as for parallel calls in one model
  step.
- A script stops after `codemode.timeoutMs`, ten minutes by default. A script can
  set a shorter deadline in its `// @options:` line, but not a longer one.
- A script can make at most `codemode.maxCalls` tool calls, 100 by default.
  Later calls fail in the script. Each script of a model response has its own
  limit.
- Runling refuses a script whose tool call has no unique ID from the model
  provider, because the calls in it could not be told apart from direct calls.
- A script receives only the text of a tool result. Images, `details`, and
  `terminate` do not reach it. Register tools that return images or end the
  turn with `exposure: 'model-only'`, so that the model calls them directly.
- Agent logs show each call from a script. Agent activity reports the script as
  one `codemode` call, so failed calls in a script do not count separately
  toward task failure limits.
- When the output of a script is longer than its output limit, Pi writes the
  complete output to a file in the temporary directory and names the file in the
  result. For an agent without the `read` tool, Runling deletes the file at once
  and removes its name from the result. Otherwise, Runling deletes the file when
  the agent ends. Set `TMPDIR` to a private directory when tool results are
  sensitive.
- `store()` keeps small values for later scripts of the same agent. With
  `sessionFile`, the values stay after a restart. A fork does not get them.
- Without the option, agents have no `codemode` tool.

## Design Decisions

### 1. Use Pi's codemode tool

**Decision:** Load Pi's `createCodemodeExtension` instead of a Runling sandbox.
**Why:** Pi maintains the sandbox, its limits, and the tool contract, and its
nested calls run through the normal tool pipeline.
**Tradeoff:** The script language and limits follow Pi.

### 2. Keep scripts inside the agent's tool set

**Decision:** Scripts call only the tools that the agent already has, and not
`report_outcome` or `models`.
**Why:** Codemode changes how the model calls tools, not what the agent may do.
The owner of the agent chooses its tools and models.
**Tradeoff:** Applications that want scripts to run classifier or image models
must add a tool for it.

## Related

- [FDR-006: Tool blocking after untrusted content](FDR-006-untrusted-tool-blocking.md)
- [FDR-007: Authorization classifier](FDR-007-authorization-classifier.md)
- [ADR-005: Block tools after untrusted context](../adr/ADR-005-untrusted-context.md)
- [Agent API guide](../agents.md#codemode)

## Open Questions

- Should Runling support MCP servers, whose tools Pi exposes to scripts?
